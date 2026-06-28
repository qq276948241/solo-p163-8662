package cron

import (
	"log"
	"time"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/handlers"
	"github.com/clinic/appointment/internal/models"
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

	processed := 0
	for _, appointment := range expiredAppointments {
		prevStatus, err := handlers.ExpireAppointment(tx, appointment.ID)
		if err != nil {
			log.Printf("Cron: Failed to expire appointment %d: %v", appointment.ID, err)
			continue
		}

		var schedule models.Schedule
		if getErr := tx.First(&schedule, appointment.ScheduleID).Error; getErr == nil {
			log.Printf("Cron: Appointment %d expired (status: %q→expired), schedule %d count: %d",
				appointment.ID, prevStatus, schedule.ID, schedule.CurrentCount)
		} else {
			log.Printf("Cron: Appointment %d expired (status: %q→expired",
				appointment.ID, prevStatus)
		}
		processed++
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Cron: Failed to commit transaction: %v", err)
		return
	}

	log.Printf("Cron: Successfully processed %d/%d expired appointments", processed, len(expiredAppointments))
}
