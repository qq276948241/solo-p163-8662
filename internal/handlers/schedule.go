package handlers

import (
	"time"

	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
)

type CreateScheduleRequest struct {
	Date            string `json:"date" binding:"required"`
	StartTime       string `json:"start_time" binding:"required"`
	EndTime         string `json:"end_time" binding:"required"`
	MaxAppointments int    `json:"max_appointments"`
}

type UpdateScheduleRequest struct {
	Date            string `json:"date"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	MaxAppointments int    `json:"max_appointments"`
	Status          string `json:"status"`
}

func CreateSchedule(c *gin.Context) {
	userID := c.GetUint("userID")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	var req CreateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		response.BadRequest(c, "Invalid date format, use YYYY-MM-DD")
		return
	}

	var existingSchedule models.Schedule
	result := database.DB.Where("doctor_id = ? AND date = ? AND start_time = ? AND end_time = ?",
		doctor.ID, date, req.StartTime, req.EndTime).First(&existingSchedule)
	if result.RowsAffected > 0 {
		response.BadRequest(c, "Schedule already exists for this time slot")
		return
	}

	maxAppointments := req.MaxAppointments
	if maxAppointments <= 0 {
		maxAppointments = 10
	}

	schedule := models.Schedule{
		DoctorID:        doctor.ID,
		Date:            date,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		MaxAppointments: maxAppointments,
		CurrentCount:    0,
		Status:          models.ScheduleStatusAvailable,
	}

	if err := database.DB.Create(&schedule).Error; err != nil {
		response.InternalServerError(c, "Failed to create schedule: "+err.Error())
		return
	}

	response.Success(c, schedule)
}

func GetMySchedules(c *gin.Context) {
	userID := c.GetUint("userID")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	date := c.Query("date")
	status := c.Query("status")

	var schedules []models.Schedule
	query := database.DB.Where("doctor_id = ?", doctor.ID).Preload("Doctor.User")

	if date != "" {
		parsedDate, err := time.Parse("2006-01-02", date)
		if err == nil {
			query = query.Where("date = ?", parsedDate)
		}
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("date ASC, start_time ASC").Find(&schedules).Error; err != nil {
		response.InternalServerError(c, "Failed to get schedules: "+err.Error())
		return
	}

	response.Success(c, schedules)
}

func GetDoctorSchedules(c *gin.Context) {
	doctorID := c.Param("doctor_id")
	date := c.Query("date")

	var schedules []models.Schedule
	query := database.DB.Where("doctor_id = ?", doctorID).Preload("Doctor.User")

	if date != "" {
		parsedDate, err := time.Parse("2006-01-02", date)
		if err == nil {
			query = query.Where("date = ?", parsedDate)
		}
	}

	query = query.Where("status = ?", models.ScheduleStatusAvailable)

	if err := query.Order("date ASC, start_time ASC").Find(&schedules).Error; err != nil {
		response.InternalServerError(c, "Failed to get schedules: "+err.Error())
		return
	}

	response.Success(c, schedules)
}

func UpdateSchedule(c *gin.Context) {
	userID := c.GetUint("userID")
	scheduleID := c.Param("id")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	var schedule models.Schedule
	if err := database.DB.Where("id = ? AND doctor_id = ?", scheduleID, doctor.ID).First(&schedule).Error; err != nil {
		response.NotFound(c, "Schedule not found")
		return
	}

	var req UpdateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	updates := make(map[string]interface{})
	if req.Date != "" {
		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			response.BadRequest(c, "Invalid date format, use YYYY-MM-DD")
			return
		}
		updates["date"] = date
	}
	if req.StartTime != "" {
		updates["start_time"] = req.StartTime
	}
	if req.EndTime != "" {
		updates["end_time"] = req.EndTime
	}
	if req.MaxAppointments > 0 {
		updates["max_appointments"] = req.MaxAppointments
	}
	if req.Status != "" {
		if req.Status != models.ScheduleStatusAvailable && req.Status != models.ScheduleStatusFull {
			response.BadRequest(c, "Invalid status")
			return
		}
		updates["status"] = req.Status
	}

	if err := database.DB.Model(&schedule).Updates(updates).Error; err != nil {
		response.InternalServerError(c, "Failed to update schedule: "+err.Error())
		return
	}

	response.Success(c, schedule)
}

func DeleteSchedule(c *gin.Context) {
	userID := c.GetUint("userID")
	scheduleID := c.Param("id")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	var schedule models.Schedule
	if err := database.DB.Where("id = ? AND doctor_id = ?", scheduleID, doctor.ID).First(&schedule).Error; err != nil {
		response.NotFound(c, "Schedule not found")
		return
	}

	var pendingAppointments int64
	database.DB.Model(&models.Appointment{}).Where("schedule_id = ? AND status IN (?)",
		scheduleID, []string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed}).Count(&pendingAppointments)

	if pendingAppointments > 0 {
		response.BadRequest(c, "Cannot delete schedule with existing appointments")
		return
	}

	if err := database.DB.Delete(&schedule).Error; err != nil {
		response.InternalServerError(c, "Failed to delete schedule: "+err.Error())
		return
	}

	response.Success(c, nil)
}
