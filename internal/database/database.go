package database

import (
	"fmt"
	"log"

	"github.com/clinic/appointment/internal/config"
	"github.com/clinic/appointment/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	var err error

	db, err := gorm.Open(mysql.Open(config.GetDSNWithoutDB()), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to MySQL server: %v", err)
	}

	createDBQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", config.AppConfig.DBName)
	if err := db.Exec(createDBQuery).Error; err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	DB, err = gorm.Open(mysql.Open(config.GetDSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Database connection established")

	err = DB.AutoMigrate(
		&models.User{},
		&models.Doctor{},
		&models.Patient{},
		&models.Schedule{},
		&models.Appointment{},
		&models.MedicalRecord{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	log.Println("Database migration completed")

	createIndexes()
}

func createIndexes() {
	DB.Exec("CREATE INDEX idx_schedules_doctor_date ON schedules(doctor_id, date)")
	DB.Exec("CREATE INDEX idx_appointments_status_expire ON appointments(status, expire_at)")
	DB.Exec("CREATE INDEX idx_appointments_patient ON appointments(patient_id)")
	DB.Exec("CREATE INDEX idx_appointments_doctor ON appointments(doctor_id)")
	DB.Exec("CREATE INDEX idx_medical_records_patient ON medical_records(patient_id)")
	DB.Exec("CREATE INDEX idx_medical_records_doctor ON medical_records(doctor_id)")
}
