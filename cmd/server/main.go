package main

import (
	"log"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/cron"
	"github.com/clinic/appointment/internal/database"
	"github.com/clinic/appointment/internal/handlers"
	"github.com/clinic/appointment/internal/middleware"
	"github.com/clinic/appointment/internal/models"
	"github.com/gin-gonic/gin"
)

func main() {
	config.LoadConfig()

	database.InitDB()

	cron.StartExpiredAppointmentCleaner()

	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", handlers.Register)
			auth.POST("/login", handlers.Login)
			auth.GET("/me", middleware.AuthMiddleware(), handlers.GetCurrentUser)
		}

		doctors := api.Group("/doctors")
		{
			doctors.GET("", handlers.GetDoctorList)
			doctors.GET("/:id", handlers.GetDoctor)
			doctors.GET("/:id/schedules", handlers.GetDoctorSchedules)

			doctorAuth := doctors.Group("")
			doctorAuth.Use(middleware.AuthMiddleware(), middleware.RoleMiddleware(models.RoleDoctor, models.RoleAdmin))
			{
				doctorAuth.GET("/me/profile", handlers.GetMyDoctorProfile)
				doctorAuth.PUT("/me/profile", handlers.UpdateMyDoctorProfile)
			}
		}

		schedules := api.Group("/schedules")
		schedules.Use(middleware.AuthMiddleware(), middleware.RoleMiddleware(models.RoleDoctor, models.RoleAdmin))
		{
			schedules.POST("", handlers.CreateSchedule)
			schedules.GET("", handlers.GetMySchedules)
			schedules.PUT("/:id", handlers.UpdateSchedule)
			schedules.DELETE("/:id", handlers.DeleteSchedule)
		}

		appointments := api.Group("/appointments")
		appointments.Use(middleware.AuthMiddleware())
		{
			appointments.POST("", middleware.RoleMiddleware(models.RolePatient, models.RoleAdmin), handlers.CreateAppointment)
			appointments.GET("", handlers.GetMyAppointments)
			appointments.GET("/:id", handlers.GetAppointment)
			appointments.POST("/:id/confirm", middleware.RoleMiddleware(models.RolePatient, models.RoleAdmin), handlers.ConfirmAppointment)
			appointments.POST("/:id/cancel", middleware.RoleMiddleware(models.RolePatient, models.RoleAdmin), handlers.CancelAppointment)
		}

		medicalRecords := api.Group("/medical-records")
		medicalRecords.Use(middleware.AuthMiddleware())
		{
			medicalRecords.POST("", middleware.RoleMiddleware(models.RoleDoctor, models.RoleAdmin), handlers.CreateMedicalRecord)
			medicalRecords.GET("", handlers.GetMyMedicalRecords)
			medicalRecords.GET("/:id", handlers.GetMedicalRecord)
			medicalRecords.GET("/patient/:patient_id", middleware.RoleMiddleware(models.RoleDoctor, models.RoleAdmin), handlers.GetPatientMedicalRecords)
		}

		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status":  "ok",
				"message": "Clinic Appointment System API is running",
			})
		})
	}

	addr := config.AppConfig.ServerHost + ":" + config.AppConfig.ServerPort
	log.Printf("Server starting on %s", addr)
	log.Printf("API Documentation:")
	log.Printf("  POST   /api/auth/register       - Register user")
	log.Printf("  POST   /api/auth/login          - User login")
	log.Printf("  GET    /api/auth/me             - Get current user")
	log.Printf("  GET    /api/doctors             - List doctors")
	log.Printf("  GET    /api/doctors/:id         - Get doctor detail")
	log.Printf("  GET    /api/doctors/:id/schedules - Get doctor schedules")
	log.Printf("  POST   /api/schedules           - Create schedule (doctor)")
	log.Printf("  GET    /api/schedules           - List my schedules (doctor)")
	log.Printf("  PUT    /api/schedules/:id       - Update schedule (doctor)")
	log.Printf("  DELETE /api/schedules/:id       - Delete schedule (doctor)")
	log.Printf("  POST   /api/appointments        - Create appointment (patient)")
	log.Printf("  GET    /api/appointments        - List my appointments")
	log.Printf("  POST   /api/appointments/:id/confirm - Confirm appointment")
	log.Printf("  POST   /api/appointments/:id/cancel  - Cancel appointment")
	log.Printf("  POST   /api/medical-records     - Create medical record (doctor)")
	log.Printf("  GET    /api/medical-records     - List my medical records")

	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
