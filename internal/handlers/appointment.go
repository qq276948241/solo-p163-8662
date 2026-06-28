package handlers

import (
	"errors"
	"fmt"
	"time"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	ErrSlotNotAvailable  = errors.New("this time slot is not available")
	ErrSlotFullyBooked   = errors.New("this time slot is fully booked")
	ErrInvalidTransition = errors.New("invalid appointment status transition")
	ErrAppointmentLocked = errors.New("appointment is locked by another operation")
)

type CreateAppointmentRequest struct {
	ScheduleID uint `json:"schedule_id" binding:"required"`
}

func generateAppointmentNo() string {
	now := time.Now()
	return fmt.Sprintf("APT%s%06d", now.Format("20060102150405"), time.Now().UnixNano()%1000000)
}

// ---------------------------------------------------------------------------
// 预约状态转换路径总览（所有变更统一走下方核心函数）：
//
//   (创建)  nil           → pending     CreateAppointment  → OccupySlot + Transition
//   (确认)  pending       → confirmed   ConfirmAppointment → Transition
//   (取消)  pending|confirmed → cancelled CancelAppointment  → Transition + ReleaseSlot
//   (过期)  pending       → expired     cron                → ExpireAppointment (内部 Transition + Release)
//   (完成)  confirmed     → completed   CreateMedicalRecord → CompleteAppointment (内部 Transition)
//
// ---------------------------------------------------------------------------

// OccupyScheduleSlot 占用排班号源。
// 加行锁、校验状态与名额、CurrentCount+1、必要时标记排班为 full。
// 返回: 更新后的排班、分配到的排队号、错误
func OccupyScheduleSlot(tx *gorm.DB, scheduleID uint) (*models.Schedule, int, error) {
	var schedule models.Schedule
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, scheduleID).Error; err != nil {
		return nil, 0, err
	}

	if schedule.Status != models.ScheduleStatusAvailable {
		return nil, 0, ErrSlotNotAvailable
	}
	if schedule.CurrentCount >= schedule.MaxAppointments {
		return nil, 0, ErrSlotFullyBooked
	}

	queueNumber := schedule.CurrentCount + 1
	schedule.CurrentCount = queueNumber

	if schedule.CurrentCount >= schedule.MaxAppointments {
		schedule.Status = models.ScheduleStatusFull
	}

	if err := tx.Save(&schedule).Error; err != nil {
		return nil, 0, err
	}
	return &schedule, queueNumber, nil
}

// ReleaseScheduleSlot 释放排班号源（取消/过期时调用）。
// 加行锁、CurrentCount-1（下限保护）、必要时恢复排班为 available。
// 返回: 更新后的排班、错误
func ReleaseScheduleSlot(tx *gorm.DB, scheduleID uint) (*models.Schedule, error) {
	var schedule models.Schedule
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, scheduleID).Error; err != nil {
		return nil, err
	}

	if schedule.CurrentCount > 0 {
		schedule.CurrentCount--
	}
	if schedule.CurrentCount < schedule.MaxAppointments && schedule.Status != models.ScheduleStatusAvailable {
		schedule.Status = models.ScheduleStatusAvailable
	}

	if err := tx.Save(&schedule).Error; err != nil {
		return nil, err
	}
	return &schedule, nil
}

// TransitionAppointmentStatus 通用预约状态转换。
// 加行锁、校验当前状态在 allowedFrom 内、更新到 toStatus。
// 参数 lockWhere 可选：附加的 WHERE 条件（例如 patient_id=? 校验所有权），params 为对应参数。
// 返回: 更新后的预约对象、是否实际发生了变更（已在目标状态返回 false）、错误
func TransitionAppointmentStatus(
	tx *gorm.DB,
	appointmentID uint,
	allowedFrom []string,
	toStatus string,
	lockWhere string,
	params ...interface{},
) (*models.Appointment, bool, error) {
	var appt models.Appointment

	query := tx.Set("gorm:query_option", "FOR UPDATE")
	if lockWhere != "" {
		query = query.Where(lockWhere, params...)
	}
	if err := query.First(&appt, appointmentID).Error; err != nil {
		return nil, false, err
	}

	if appt.Status == toStatus {
		return &appt, false, nil
	}

	allowed := false
	for _, s := range allowedFrom {
		if appt.Status == s {
			allowed = true
			break
		}
	}
	if !allowed {
		return &appt, false, fmt.Errorf("%w: cannot transition from %q to %q",
			ErrInvalidTransition, appt.Status, toStatus)
	}

	appt.Status = toStatus
	if err := tx.Save(&appt).Error; err != nil {
		return nil, false, err
	}
	return &appt, true, nil
}

// ExpireAppointment 过期处理：pending→expired + 释放号源。
// （cron 调用，返回原状态便于日志）
func ExpireAppointment(tx *gorm.DB, appointmentID uint) (string, error) {
	appt, changed, err := TransitionAppointmentStatus(
		tx, appointmentID,
		[]string{models.AppointmentStatusPending},
		models.AppointmentStatusExpired,
		"",
	)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		return "", err
	}
	prevStatus := appt.Status
	if !changed {
		return prevStatus, nil
	}

	if _, err := ReleaseScheduleSlot(tx, appt.ScheduleID); err != nil {
		return "", err
	}
	return models.AppointmentStatusPending, nil
}

// CompleteAppointment 完成就诊：confirmed→completed。
// （medical_record 调用，不涉及号源释放）
func CompleteAppointment(tx *gorm.DB, appointmentID uint, lockWhere string, params ...interface{}) error {
	_, _, err := TransitionAppointmentStatus(
		tx, appointmentID,
		[]string{models.AppointmentStatusConfirmed},
		models.AppointmentStatusCompleted,
		lockWhere, params...,
	)
	return err
}

// ===========================================================================
// 以下为 HTTP Handler
// ===========================================================================

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

	var existingAppointment models.Appointment
	result := tx.Where("patient_id = ? AND schedule_id = ? AND status IN (?)",
		patient.ID, req.ScheduleID,
		[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed}).
		First(&existingAppointment)
	if result.RowsAffected > 0 {
		tx.Rollback()
		response.BadRequest(c, "You already have an appointment for this time slot")
		return
	}

	schedule, queueNumber, err := OccupyScheduleSlot(tx, req.ScheduleID)
	if err != nil {
		tx.Rollback()
		switch {
		case errors.Is(err, ErrSlotNotAvailable):
			response.BadRequest(c, "This time slot is not available")
		case errors.Is(err, ErrSlotFullyBooked):
			response.BadRequest(c, "This time slot is fully booked")
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "Schedule not found")
		default:
			response.InternalServerError(c, "Failed to occupy slot: "+err.Error())
		}
		return
	}

	appointment := models.Appointment{
		PatientID:     patient.ID,
		ScheduleID:    schedule.ID,
		DoctorID:      schedule.DoctorID,
		Status:        models.AppointmentStatusPending,
		AppointmentNo: generateAppointmentNo(),
		QueueNumber:   queueNumber,
		ExpireAt:      time.Now().Add(time.Duration(config.AppConfig.AppointmentTimeoutMinutes) * time.Minute),
	}

	if err := tx.Create(&appointment).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to create appointment: "+err.Error())
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

	appt, changed, err := TransitionAppointmentStatus(
		tx, parseUintParam(appointmentID),
		[]string{models.AppointmentStatusPending},
		models.AppointmentStatusConfirmed,
		"patient_id = ?", patient.ID,
	)
	if err != nil {
		tx.Rollback()
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "Appointment not found")
		case errors.Is(err, ErrInvalidTransition):
			handleConfirmTransitionError(c, appt)
		default:
			response.InternalServerError(c, "Failed to confirm appointment: "+err.Error())
		}
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to commit transaction: "+err.Error())
		return
	}

	if changed {
		response.SuccessWithMessage(c, "Appointment confirmed", appt)
	} else {
		response.SuccessWithMessage(c, "Appointment already confirmed", appt)
	}
}

func handleConfirmTransitionError(c *gin.Context, appt *models.Appointment) {
	switch appt.Status {
	case models.AppointmentStatusExpired:
		response.BadRequest(c, "This appointment has expired")
	case models.AppointmentStatusCancelled:
		response.BadRequest(c, "This appointment has been cancelled")
	case models.AppointmentStatusCompleted:
		response.BadRequest(c, "This appointment has been completed")
	default:
		response.BadRequest(c, "Cannot confirm appointment in current status")
	}
}

func CancelAppointment(c *gin.Context) {
	userID := c.GetUint("userID")
	appointmentID := c.Param("id")

	var patient models.Patient
	if err := database.DB.Where("user_id = ?", userID).First(&patient).Error; err != nil {
		response.NotFound(c, "Patient profile not found")
		return
	}

	var preCheckAppt models.Appointment
	if err := database.DB.Where("id = ? AND patient_id = ?", parseUintParam(appointmentID), patient.ID).
		First(&preCheckAppt).Error; err != nil {
		response.NotFound(c, "Appointment not found")
		return
	}
	switch preCheckAppt.Status {
	case models.AppointmentStatusCancelled:
		response.SuccessWithMessage(c, "Appointment already cancelled, no action needed", nil)
		return
	case models.AppointmentStatusExpired:
		response.SuccessWithMessage(c, "This appointment has expired and been automatically released. The slot is now available", nil)
		return
	case models.AppointmentStatusCompleted:
		response.BadRequest(c, "This appointment has been completed and cannot be cancelled")
		return
	case models.AppointmentStatusPending, models.AppointmentStatusConfirmed:
	default:
		response.BadRequest(c, "Cannot cancel appointment in current status")
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

	appt, changed, err := TransitionAppointmentStatus(
		tx, parseUintParam(appointmentID),
		[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
		models.AppointmentStatusCancelled,
		"patient_id = ?", patient.ID,
	)
	if err != nil {
		tx.Rollback()
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "Appointment not found")
		case errors.Is(err, ErrInvalidTransition):
			handleCancelTransitionError(c, appt)
		default:
			response.InternalServerError(c, "Failed to cancel appointment: "+err.Error())
		}
		return
	}

	var schedule *models.Schedule
	if changed {
		schedule, err = ReleaseScheduleSlot(tx, appt.ScheduleID)
		if err != nil {
			tx.Rollback()
			response.InternalServerError(c, "Failed to release slot: "+err.Error())
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to commit transaction: "+err.Error())
		return
	}

	if !changed {
		switch appt.Status {
		case models.AppointmentStatusCancelled:
			response.SuccessWithMessage(c, "Appointment already cancelled by another request", nil)
		case models.AppointmentStatusExpired:
			response.SuccessWithMessage(c, "This appointment has expired and been automatically released. The slot is now available", nil)
		default:
			response.SuccessWithMessage(c, "No change needed", appt)
		}
		return
	}

	response.SuccessWithMessage(c, "Appointment cancelled successfully, slot released", gin.H{
		"appointment_id": appt.ID,
		"appointment_no": appt.AppointmentNo,
		"status":         appt.Status,
		"schedule_id":    schedule.ID,
		"slots_left":     schedule.MaxAppointments - schedule.CurrentCount,
	})
}

func handleCancelTransitionError(c *gin.Context, appt *models.Appointment) {
	switch appt.Status {
	case models.AppointmentStatusCancelled:
		response.SuccessWithMessage(c, "Appointment already cancelled by another request", nil)
	case models.AppointmentStatusExpired:
		response.SuccessWithMessage(c, "This appointment has expired and been automatically released. The slot is now available", nil)
	case models.AppointmentStatusCompleted:
		response.BadRequest(c, "This appointment has been completed and cannot be cancelled")
	default:
		response.BadRequest(c, "Cannot cancel appointment in current status")
	}
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

func parseUintParam(s string) uint {
	var n uint
	fmt.Sscanf(s, "%d", &n)
	return n
}
