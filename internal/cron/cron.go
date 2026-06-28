package cron

import (
	"log"
	"time"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"gorm.io/gorm"
)

func StartExpiredAppointmentCleaner() {
	interval := time.Duration(config.AppConfig.CronIntervalSeconds) * time.Second

	go func() {
		log.Printf("Cron job started: checking for expired appointments every %v", interval)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			cleanExpiredAppointments()
		}
	}()
}

func cleanExpiredAppointments() {
	now := time.Now()

	tx := database.DB.Begin()
	if tx.Error != nil {
		log.Printf("Cron: Failed to start transaction: %v", tx.Error)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Cron: Panic recovered: %v", r)
		}
	}()

	var expiredAppointments []models.Appointment
	if err := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").
		Where("status = ? AND expire_at < ?", models.AppointmentStatusPending, now).
		Find(&expiredAppointments).Error; err != nil {
		tx.Rollback()
		log.Printf("Cron: Failed to query expired appointments: %v", err)
		return
	}

	if len(expiredAppointments) == 0 {
		tx.Commit()
		return
	}

	log.Printf("Cron: Found %d expired appointments to process", len(expiredAppointments))

	for _, appointment := range expiredAppointments {
		if err := processExpiredAppointment(tx, appointment); err != nil {
			log.Printf("Cron: Failed to process appointment %d: %v", appointment.ID, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Cron: Failed to commit transaction: %v", err)
		return
	}

	log.Printf("Cron: Successfully processed %d expired appointments", len(expiredAppointments))
}

func processExpiredAppointment(tx *gorm.DB, appointment models.Appointment) error {
	appointment.Status = models.AppointmentStatusExpired
	if err := tx.Save(&appointment).Error; err != nil {
		return err
	}

	var schedule models.Schedule
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, appointment.ScheduleID).Error; err != nil {
		return err
	}

	schedule.CurrentCount--
	if schedule.CurrentCount < 0 {
		schedule.CurrentCount = 0
	}
	if schedule.CurrentCount < schedule.MaxAppointments && schedule.Status == models.ScheduleStatusFull {
		schedule.Status = models.ScheduleStatusAvailable
	}
	if err := tx.Save(&schedule).Error; err != nil {
		return err
	}

	log.Printf("Cron: Appointment %d expired, schedule %d count updated to %d",
		appointment.ID, schedule.ID, schedule.CurrentCount)

	return nil
}
