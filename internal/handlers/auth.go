package handlers

import (
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/models"
	"github.com/clinic/appointment/internal/utils"
	"github.com/clinic/appointment/pkg/response"
	"github.com/gin-gonic/gin"
)

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Name     string `json:"name" binding:"required"`
	Role     string `json:"role"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Gender   string `json:"gender"`
	Age      int    `json:"age"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	var existingUser models.User
	if result := database.DB.Where("username = ?", req.Username).First(&existingUser); result.RowsAffected > 0 {
		response.BadRequest(c, "Username already exists")
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		response.InternalServerError(c, "Failed to hash password")
		return
	}

	role := req.Role
	if role == "" {
		role = models.RolePatient
	}

	if role != models.RoleDoctor && role != models.RolePatient && role != models.RoleAdmin {
		response.BadRequest(c, "Invalid role, must be 'doctor', 'patient', or 'admin'")
		return
	}

	user := models.User{
		Username: req.Username,
		Password: hashedPassword,
		Name:     req.Name,
		Role:     role,
		Phone:    req.Phone,
		Email:    req.Email,
		Gender:   req.Gender,
		Age:      req.Age,
	}

	tx := database.DB.Begin()
	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		response.InternalServerError(c, "Failed to create user: "+err.Error())
		return
	}

	if role == models.RolePatient {
		patient := models.Patient{
			UserID: user.ID,
		}
		if err := tx.Create(&patient).Error; err != nil {
			tx.Rollback()
			response.InternalServerError(c, "Failed to create patient profile: "+err.Error())
			return
		}
	}

	if role == models.RoleDoctor {
		doctor := models.Doctor{
			UserID: user.ID,
		}
		if err := tx.Create(&doctor).Error; err != nil {
			tx.Rollback()
			response.InternalServerError(c, "Failed to create doctor profile: "+err.Error())
			return
		}
	}

	tx.Commit()

	response.Success(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"name":     user.Name,
		"role":     user.Role,
	})
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request parameters: "+err.Error())
		return
	}

	var user models.User
	if result := database.DB.Where("username = ?", req.Username).First(&user); result.Error != nil {
		response.Unauthorized(c, "Invalid username or password")
		return
	}

	if !utils.CheckPasswordHash(req.Password, user.Password) {
		response.Unauthorized(c, "Invalid username or password")
		return
	}

	token, err := utils.GenerateToken(user.ID, user.Role, user.Name)
	if err != nil {
		response.InternalServerError(c, "Failed to generate token")
		return
	}

	userData := gin.H{
		"id":       user.ID,
		"username": user.Username,
		"name":     user.Name,
		"role":     user.Role,
		"phone":    user.Phone,
		"email":    user.Email,
	}

	if user.Role == models.RoleDoctor {
		var doctor models.Doctor
		database.DB.Where("user_id = ?", user.ID).First(&doctor)
		userData["doctor_id"] = doctor.ID
		userData["department"] = doctor.Department
		userData["title"] = doctor.Title
	}

	if user.Role == models.RolePatient {
		var patient models.Patient
		database.DB.Where("user_id = ?", user.ID).First(&patient)
		userData["patient_id"] = patient.ID
	}

	response.Success(c, LoginResponse{
		Token: token,
		User:  userData,
	})
}

func GetCurrentUser(c *gin.Context) {
	userID := c.GetUint("userID")
	role := c.GetString("role")
	name := c.GetString("name")

	response.Success(c, gin.H{
		"user_id": userID,
		"role":    role,
		"name":    name,
	})
}
