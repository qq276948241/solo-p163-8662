package models

import (
	"time"
)

const (
	RoleDoctor  = "doctor"
	RolePatient = "patient"
	RoleAdmin   = "admin"
)

const (
	ScheduleStatusAvailable = "available"
	ScheduleStatusFull      = "full"
)

const (
	AppointmentStatusPending   = "pending"
	AppointmentStatusConfirmed = "confirmed"
	AppointmentStatusCancelled = "cancelled"
	AppointmentStatusCompleted = "completed"
	AppointmentStatusExpired   = "expired"
)

type User struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username  string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"type:varchar(255);not null" json:"-"`
	Name      string    `gorm:"type:varchar(50);not null" json:"name"`
	Role      string    `gorm:"type:varchar(20);not null;default:'patient'" json:"role"`
	Phone     string    `gorm:"type:varchar(20)" json:"phone"`
	Email     string    `gorm:"type:varchar(100)" json:"email"`
	Gender    string    `gorm:"type:varchar(10)" json:"gender"`
	Age       int       `gorm:"type:int" json:"age"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Doctor struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        uint      `gorm:"not null;index" json:"user_id"`
	User          User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Department    string    `gorm:"type:varchar(50);not null" json:"department"`
	Title         string    `gorm:"type:varchar(50)" json:"title"`
	Specialty     string    `gorm:"type:varchar(200)" json:"specialty"`
	Description   string    `gorm:"type:text" json:"description"`
	ConsultationFee float64 `gorm:"type:decimal(10,2);default:0" json:"consultation_fee"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Patient struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	IDCard      string    `gorm:"type:varchar(18);uniqueIndex" json:"id_card"`
	Address     string    `gorm:"type:varchar(255)" json:"address"`
	MedicalHistory string `gorm:"type:text" json:"medical_history"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Schedule struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	DoctorID        uint      `gorm:"not null;index" json:"doctor_id"`
	Doctor          Doctor    `gorm:"foreignKey:DoctorID" json:"doctor,omitempty"`
	Date            time.Time `gorm:"type:date;not null;index" json:"date"`
	StartTime       string    `gorm:"type:varchar(10);not null" json:"start_time"`
	EndTime         string    `gorm:"type:varchar(10);not null" json:"end_time"`
	MaxAppointments int       `gorm:"not null;default:10" json:"max_appointments"`
	CurrentCount    int       `gorm:"not null;default:0" json:"current_count"`
	Status          string    `gorm:"type:varchar(20);default:'available'" json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Appointment struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PatientID     uint      `gorm:"not null;index" json:"patient_id"`
	Patient       Patient   `gorm:"foreignKey:PatientID" json:"patient,omitempty"`
	ScheduleID    uint      `gorm:"not null;index" json:"schedule_id"`
	Schedule      Schedule  `gorm:"foreignKey:ScheduleID" json:"schedule,omitempty"`
	DoctorID      uint      `gorm:"not null;index" json:"doctor_id"`
	Doctor        Doctor    `gorm:"foreignKey:DoctorID" json:"doctor,omitempty"`
	Status        string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	AppointmentNo string    `gorm:"type:varchar(32);uniqueIndex" json:"appointment_no"`
	QueueNumber   int       `gorm:"type:int" json:"queue_number"`
	ExpireAt      time.Time `gorm:"index" json:"expire_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MedicalRecord struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	AppointmentID uint      `gorm:"not null;index" json:"appointment_id"`
	Appointment   Appointment `gorm:"foreignKey:AppointmentID" json:"appointment,omitempty"`
	PatientID     uint      `gorm:"not null;index" json:"patient_id"`
	Patient       Patient   `gorm:"foreignKey:PatientID" json:"patient,omitempty"`
	DoctorID      uint      `gorm:"not null;index" json:"doctor_id"`
	Doctor        Doctor    `gorm:"foreignKey:DoctorID" json:"doctor,omitempty"`
	Diagnosis     string    `gorm:"type:text;not null" json:"diagnosis"`
	Prescription  string    `gorm:"type:text" json:"prescription"`
	Advice        string    `gorm:"type:text" json:"advice"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
