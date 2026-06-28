package handlers

import (
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CreateMedicalRecordRequest struct {
	AppointmentID uint   `json:"appointment_id" binding:"required"`
	Diagnosis     string `json:"diagnosis" binding:"required"`
	Prescription  string `json:"prescription"`
	Advice        string `json:"advice"`
}

func CreateMedicalRecord(c *gin.Context) {
	userID := c.GetUint("userID")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	var req CreateMedicalRecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		response.InternalServerError(c, "Failed to start transaction")
		return
	}

	var appointment models.Appointment
	if err := tx.Where("id = ? AND doctor_id = ?", req.AppointmentID, doctor.ID).First(&appointment).Error; err != nil {
		tx.Rollback()
		response.NotFound(c, "Appointment not found")
		return
	}

	if appointment.Status != models.AppointmentStatusConfirmed {
		tx.Rollback()
		response.BadRequest(c, "Appointment is not confirmed")
		return
	}

	var existingRecord models.MedicalRecord
	result := tx.Where("appointment_id = ?", req.AppointmentID).First(&existingRecord)
	if result.RowsAffected > 0 {
		tx.Rollback()
		response.BadRequest(c, "Medical record already exists for this appointment")
		return
	}

	medicalRecord := models.MedicalRecord{
		AppointmentID: req.AppointmentID,
		PatientID:     appointment.PatientID,
		DoctorID:      doctor.ID,
		Diagnosis:     req.Diagnosis,
		Prescription:  req.Prescription,
		Advice:        req.Advice,
	}

	if err := tx.Create(&medicalRecord).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to create medical record: "+err.Error())
		return
	}

	appointment.Status = models.AppointmentStatusCompleted
	if err := tx.Save(&appointment).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to update appointment status: "+err.Error())
		return
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to commit transaction: "+err.Error())
		return
	}

	response.Success(c, medicalRecord)
}

func GetMyMedicalRecords(c *gin.Context) {
	userID := c.GetUint("userID")
	role := c.GetString("role")

	var records []models.MedicalRecord
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
		query = database.DB.Model(&models.MedicalRecord{})
	}

	if err := query.Preload("Appointment.Schedule").Preload("Doctor.User").Preload("Patient.User").
		Order("created_at DESC").Find(&records).Error; err != nil {
		response.InternalServerError(c, "Failed to get medical records: "+err.Error())
		return
	}

	response.Success(c, records)
}

func GetMedicalRecord(c *gin.Context) {
	userID := c.GetUint("userID")
	recordID := c.Param("id")
	role := c.GetString("role")

	var record models.MedicalRecord
	query := database.DB.Preload("Appointment.Schedule").Preload("Doctor.User").Preload("Patient.User")

	if role == models.RolePatient {
		var patient models.Patient
		database.DB.Where("user_id = ?", userID).First(&patient)
		query = query.Where("id = ? AND patient_id = ?", recordID, patient.ID)
	} else if role == models.RoleDoctor {
		var doctor models.Doctor
		database.DB.Where("user_id = ?", userID).First(&doctor)
		query = query.Where("id = ? AND doctor_id = ?", recordID, doctor.ID)
	} else {
		query = query.Where("id = ?", recordID)
	}

	if err := query.First(&record).Error; err != nil {
		response.NotFound(c, "Medical record not found")
		return
	}

	response.Success(c, record)
}

func GetPatientMedicalRecords(c *gin.Context) {
	patientID := c.Param("patient_id")

	var records []models.MedicalRecord
	if err := database.DB.Where("patient_id = ?", patientID).
		Preload("Appointment.Schedule").Preload("Doctor.User").Preload("Patient.User").
		Order("created_at DESC").Find(&records).Error; err != nil {
		response.InternalServerError(c, "Failed to get medical records: "+err.Error())
		return
	}

	response.Success(c, records)
}
