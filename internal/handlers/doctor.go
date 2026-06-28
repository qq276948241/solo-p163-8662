package handlers

import (
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
)

type UpdateDoctorRequest struct {
	Department      string  `json:"department"`
	Title           string  `json:"title"`
	Specialty       string  `json:"specialty"`
	Description     string  `json:"description"`
	ConsultationFee float64 `json:"consultation_fee"`
}

func GetDoctorList(c *gin.Context) {
	department := c.Query("department")

	var doctors []models.Doctor
	query := database.DB.Preload("User")

	if department != "" {
		query = query.Where("department = ?", department)
	}

	if err := query.Find(&doctors).Error; err != nil {
		response.InternalServerError(c, "Failed to get doctor list: "+err.Error())
		return
	}

	response.Success(c, doctors)
}

func GetDoctor(c *gin.Context) {
	id := c.Param("id")

	var doctor models.Doctor
	if err := database.DB.Preload("User").First(&doctor, id).Error; err != nil {
		response.NotFound(c, "Doctor not found")
		return
	}

	response.Success(c, doctor)
}

func GetMyDoctorProfile(c *gin.Context) {
	userID := c.GetUint("userID")

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).Preload("User").First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	response.Success(c, doctor)
}

func UpdateMyDoctorProfile(c *gin.Context) {
	userID := c.GetUint("userID")

	var req UpdateDoctorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	var doctor models.Doctor
	if err := database.DB.Where("user_id = ?", userID).First(&doctor).Error; err != nil {
		response.NotFound(c, "Doctor profile not found")
		return
	}

	updates := make(map[string]interface{})
	if req.Department != "" {
		updates["department"] = req.Department
	}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Specialty != "" {
		updates["specialty"] = req.Specialty
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.ConsultationFee > 0 {
		updates["consultation_fee"] = req.ConsultationFee
	}

	if err := database.DB.Model(&doctor).Updates(updates).Error; err != nil {
		response.InternalServerError(c, "Failed to update doctor profile: "+err.Error())
		return
	}

	response.Success(c, doctor)
}
