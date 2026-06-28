package handlers

import (
	"fmt"
	"time"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CreateAppointmentRequest struct {
	ScheduleID uint `json:"schedule_id" binding:"required"`
}

func generateAppointmentNo() string {
	now := time.Now()
	return fmt.Sprintf("APT%s%06d", now.Format("20060102150405"), time.Now().UnixNano()%1000000)
}

func CreateAppointment(c *gin.Context) {
	userID := c.GetUint("userID")

	var patient models.Patient
	if err := database.DB.Where("user_id = ?", userID).First(&patient).Error; err != nil {
		response.NotFound(c, "Patient profile not found")
		return
	}

	var req CreateAppointmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		response.InternalServerError(c, "Failed to start transaction")
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var schedule models.Schedule
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, req.ScheduleID).Error; err != nil {
		tx.Rollback()
		response.NotFound(c, "Schedule not found")
		return
	}

	if schedule.Status != models.ScheduleStatusAvailable {
		tx.Rollback()
		response.BadRequest(c, "This time slot is not available")
		return
	}

	if schedule.CurrentCount >= schedule.MaxAppointments {
		tx.Rollback()
		response.BadRequest(c, "This time slot is fully booked")
		return
	}

	var existingAppointment models.Appointment
	result := tx.Where("patient_id = ? AND schedule_id = ? AND status IN (?)",
		patient.ID, req.ScheduleID, []string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed}).First(&existingAppointment)
	if result.RowsAffected > 0 {
		tx.Rollback()
		response.BadRequest(c, "You already have an appointment for this time slot")
		return
	}

	queueNumber := schedule.CurrentCount + 1
	appointmentNo := generateAppointmentNo()
	expireAt := time.Now().Add(time.Duration(config.AppConfig.AppointmentTimeoutMinutes) * time.Minute)

	appointment := models.Appointment{
		PatientID:     patient.ID,
		ScheduleID:    schedule.ID,
		DoctorID:      schedule.DoctorID,
		Status:        models.AppointmentStatusPending,
		AppointmentNo: appointmentNo,
		QueueNumber:   queueNumber,
		ExpireAt:      expireAt,
	}

	if err := tx.Create(&appointment).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to create appointment: "+err.Error())
		return
	}

	schedule.CurrentCount = queueNumber
	if schedule.CurrentCount >= schedule.MaxAppointments {
		schedule.Status = models.ScheduleStatusFull
	}
	if err := tx.Save(&schedule).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to update schedule: "+err.Error())
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to commit transaction: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"appointment_id":  appointment.ID,
		"appointment_no":  appointment.AppointmentNo,
		"queue_number":    appointment.QueueNumber,
		"status":          appointment.Status,
		"expire_at":       appointment.ExpireAt,
		"schedule_id":     schedule.ID,
		"doctor_id":       schedule.DoctorID,
		"date":            schedule.Date.Format("2006-01-02"),
		"start_time":      schedule.StartTime,
		"end_time":        schedule.EndTime,
	})
}

func ConfirmAppointment(c *gin.Context) {
	userID := c.GetUint("userID")
	appointmentID := c.Param("id")

	var patient models.Patient
	if err := database.DB.Where("user_id = ?", userID).First(&patient).Error; err != nil {
		response.NotFound(c, "Patient profile not found")
		return
	}

	var appointment models.Appointment
	if err := database.DB.Where("id = ? AND patient_id = ?", appointmentID, patient.ID).First(&appointment).Error; err != nil {
		response.NotFound(c, "Appointment not found")
		return
	}

	if appointment.Status == models.AppointmentStatusExpired {
		response.BadRequest(c, "This appointment has expired")
		return
	}

	if appointment.Status == models.AppointmentStatusCancelled {
		response.BadRequest(c, "This appointment has been cancelled")
		return
	}

	if appointment.Status == models.AppointmentStatusConfirmed {
		response.Success(c, appointment)
		return
	}

	if appointment.Status != models.AppointmentStatusPending {
		response.BadRequest(c, "Invalid appointment status")
		return
	}

	appointment.Status = models.AppointmentStatusConfirmed
	if err := database.DB.Save(&appointment).Error; err != nil {
		response.InternalServerError(c, "Failed to confirm appointment: "+err.Error())
		return
	}

	response.Success(c, appointment)
}

func CancelAppointment(c *gin.Context) {
	userID := c.GetUint("userID")
	appointmentID := c.Param("id")

	var patient models.Patient
	if err := database.DB.Where("user_id = ?", userID).First(&patient).Error; err != nil {
		response.NotFound(c, "Patient profile not found")
		return
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		response.InternalServerError(c, "Failed to start transaction")
		return
	}

	var appointment models.Appointment
	if err := tx.Where("id = ? AND patient_id = ?", appointmentID, patient.ID).First(&appointment).Error; err != nil {
		tx.Rollback()
		response.NotFound(c, "Appointment not found")
		return
	}

	if appointment.Status == models.AppointmentStatusCancelled || appointment.Status == models.AppointmentStatusCompleted {
		tx.Rollback()
		response.BadRequest(c, "Cannot cancel this appointment")
		return
	}

	appointment.Status = models.AppointmentStatusCancelled
	if err := tx.Save(&appointment).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to cancel appointment: "+err.Error())
		return
	}

	var schedule models.Schedule
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, appointment.ScheduleID).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to get schedule: "+err.Error())
		return
	}

	schedule.CurrentCount--
	if schedule.CurrentCount < 0 {
		schedule.CurrentCount = 0
	}
	if schedule.CurrentCount < schedule.MaxAppointments {
		schedule.Status = models.ScheduleStatusAvailable
	}
	if err := tx.Save(&schedule).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to update schedule: "+err.Error())
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to commit transaction: "+err.Error())
		return
	}

	response.Success(c, nil)
}

func GetMyAppointments(c *gin.Context) {
	userID := c.GetUint("userID")
	role := c.GetString("role")

	status := c.Query("status")

	var appointments []models.Appointment
	var query *gorm.DB

	if role == models.RolePatient {
		var patient models.Patient
		if err := database.DB.Where("user_id = ?", userID).First(&patient).Error; err != nil {
			response.NotFound(c, "Patient profile not found")
			return
		}
		query = database.DB.Where("patient_id = ?", patient.ID)
	} else if role == models.RoleDoctor {
		var doctor models.Doctor
		if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
			response.NotFound(c, "Doctor profile not found")
			return
		}
		query = database.DB.Where("doctor_id = ?", doctor.ID)
	} else {
		query = database.DB.Model(&models.Appointment{})
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Preload("Schedule").Preload("Doctor.User").Preload("Patient.User").
		Order("created_at DESC").Find(&appointments).Error; err != nil {
		response.InternalServerError(c, "Failed to get appointments: "+err.Error())
		return
	}

	response.Success(c, appointments)
}

func GetAppointment(c *gin.Context) {
	userID := c.GetUint("userID")
	appointmentID := c.Param("id")
	role := c.GetString("role")

	var appointment models.Appointment
	query := database.DB.Preload("Schedule").Preload("Doctor.User").Preload("Patient.User")

	if role == models.RolePatient {
		var patient models.Patient
		database.DB.Where("user_id = ?", userID).First(&patient)
		query = query.Where("id = ? AND patient_id = ?", appointmentID, patient.ID)
	} else if role == models.RoleDoctor {
		var doctor models.Doctor
		database.DB.Where("user_id = ?", userID).First(&doctor)
		query = query.Where("id = ? AND doctor_id = ?", appointmentID, doctor.ID)
	} else {
		query = query.Where("id = ?", appointmentID)
	}

	if err := query.First(&appointment).Error; err != nil {
		response.NotFound(c, "Appointment not found")
		return
	}

	response.Success(c, appointment)
}
