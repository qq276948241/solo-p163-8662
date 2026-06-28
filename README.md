# Clinic Appointment System (社区诊所预约挂号系统)

A complete backend system for community clinic appointment registration, built with Go + Gin + MySQL.

## Features

### 1. Doctor Scheduling Module (医生排班)
- Doctor profile management (department, title, specialty, consultation fee)
- Create, read, update, delete schedule time slots
- Schedule availability management
- Maximum appointments per time slot configuration

### 2. Patient Appointment Module (患者预约)
- Patient browse doctors and available schedules
- Appointment booking with **row lock transaction** to prevent double booking
- Appointment status: pending → confirmed → completed / cancelled / expired
- **15-minute auto-release** for unconfirmed appointments (cron job)
- Appointment confirmation and cancellation

### 3. Medical Record Module (就诊记录)
- Doctor creates medical records after consultation
- Includes diagnosis, prescription, and medical advice
- Appointment status automatically updated to "completed"
- Patient can view their own medical records

## Technical Features

- **RESTful API** returning standard JSON format
- **JWT** authentication with role-based access control (doctor/patient/admin)
- **bcrypt** password encryption
- **GORM ORM** with MySQL auto-migration
- **Database row lock** (`SELECT ... FOR UPDATE`) for concurrent appointment booking
- **Cron job** for automatic expired appointment cleanup
- **CORS** support for frontend integration

## Project Structure

```
project163/
├── cmd/server/          # Server entry point
│   └── main.go
├── internal/
│   ├── config/          # Configuration loading
│   ├── models/          # Database models
│   ├── database/        # Database connection & migration
│   ├── middleware/      # Auth middleware
│   ├── utils/           # JWT & password utilities
│   ├── handlers/        # API handlers
│   └── cron/            # Cron jobs
├── pkg/response/        # Standard response format
├── .env                 # Environment variables
├── test_flow.ps1        # End-to-end test script
└── go.mod
```

## API Endpoints

### Auth
- `POST /api/auth/register` - Register user (doctor/patient)
- `POST /api/auth/login` - User login, returns JWT token
- `GET /api/auth/me` - Get current user info

### Doctors (Public)
- `GET /api/doctors` - List all doctors (filter by department)
- `GET /api/doctors/:id` - Get doctor detail
- `GET /api/doctors/:id/schedules` - Get doctor's available schedules

### Doctors (Authenticated, Doctor Role)
- `GET /api/doctors/me/profile` - Get my doctor profile
- `PUT /api/doctors/me/profile` - Update my doctor profile

### Schedules (Doctor Role)
- `POST /api/schedules` - Create new schedule
- `GET /api/schedules` - List my schedules
- `PUT /api/schedules/:id` - Update schedule
- `DELETE /api/schedules/:id` - Delete schedule

### Appointments (Authenticated)
- `POST /api/appointments` - Create appointment (Patient Role)
- `GET /api/appointments` - List my appointments
- `GET /api/appointments/:id` - Get appointment detail
- `POST /api/appointments/:id/confirm` - Confirm appointment (Patient)
- `POST /api/appointments/:id/cancel` - Cancel appointment (Patient)

### Medical Records (Authenticated)
- `POST /api/medical-records` - Create medical record (Doctor Role)
- `GET /api/medical-records` - List my medical records
- `GET /api/medical-records/:id` - Get medical record detail
- `GET /api/medical-records/patient/:patient_id` - Get patient's records (Doctor)

## Quick Start

### Prerequisites
- Go 1.20+
- MySQL 5.7+ or 8.0+

### 1. Configure Database

Edit `.env` file:
```env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=clinic_appointment
```

### 2. Install Dependencies & Build

```bash
go mod tidy
go build -o bin/server.exe ./cmd/server
```

### 3. Start Server

```bash
./bin/server.exe
```

The server will start on `http://localhost:8080`

The database and tables will be created automatically on first run.

### 4. Run End-to-End Test

```powershell
.\test_flow.ps1
```

This will test the complete flow:
1. Register doctor account
2. Doctor login
3. Update doctor profile
4. Create schedule time slots
5. Register patient account
6. Patient login
7. Patient book appointment
8. Patient confirm appointment
9. Doctor create medical record
10. Patient view medical records

## Key Technical Details

### 1. Row Lock for Concurrent Booking

In [appointment.go](internal/handlers/appointment.go#L51), we use database row lock to prevent overbooking:

```go
var schedule models.Schedule
if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&schedule, req.ScheduleID).Error; err != nil {
    // handle error
}
```

This ensures that when multiple patients try to book the same time slot simultaneously, only one can proceed at a time.

### 2. 15-Minute Auto-Release Cron Job

In [cron.go](internal/cron/cron.go), a background goroutine runs every 60 seconds to check for expired appointments:

```go
ticker := time.NewTicker(interval)
for range ticker.C {
    cleanExpiredAppointments()
}
```

Expired appointments (pending status + expire_at < now) are automatically marked as "expired" and the schedule count is released.

### 3. JWT Authentication

Token contains user ID, role, and name. Middleware in [auth.go](internal/middleware/auth.go) validates tokens and enforces role-based access.

### 4. Password Security

All passwords are hashed using bcrypt with default cost before storage in [password.go](internal/utils/password.go).

## Database Tables

- `users` - User accounts (doctors, patients, admins)
- `doctors` - Doctor profiles (linked to users)
- `patients` - Patient profiles (linked to users)
- `schedules` - Doctor scheduling time slots
- `appointments` - Patient appointment records
- `medical_records` - Consultation records with diagnosis & prescription

## Response Format

All APIs return consistent JSON format:

```json
{
    "code": 200,
    "message": "success",
    "data": { ... }
}
```
