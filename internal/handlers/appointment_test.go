package handlers

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/clinic/appointment/internal/models"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite via modernc: %v", err)
	}
	db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Doctor{}, &models.Patient{},
		&models.Schedule{}, &models.Appointment{}, &models.MedicalRecord{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func seedPatient(db *gorm.DB) *models.Patient {
	u := &models.User{Username: fmt.Sprintf("p_%d", time.Now().UnixNano()), Password: "x", Role: models.RolePatient, Name: "PT", Phone: "1", Gender: "M", Age: 30}
	db.Create(u)
	p := &models.Patient{UserID: u.ID}
	db.Create(p)
	return p
}

func seedSchedule(db *gorm.DB, maxApts int) *models.Schedule {
	s := &models.Schedule{
		DoctorID:        1,
		Date:            time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		StartTime:       "10:00",
		EndTime:         "11:00",
		MaxAppointments: maxApts,
		CurrentCount:    0,
		Status:          models.ScheduleStatusAvailable,
	}
	db.Create(s)
	return s
}

func seedAppointment(db *gorm.DB, patientID, scheduleID uint, status string) *models.Appointment {
	a := &models.Appointment{
		PatientID:    patientID,
		ScheduleID:   scheduleID,
		DoctorID:     1,
		AppointmentNo: fmt.Sprintf("A_%d_%d", scheduleID, time.Now().UnixNano()),
		QueueNumber:  1,
		Status:       status,
		ExpireAt:     time.Now().Add(time.Hour),
	}
	db.Create(a)
	return a
}

// TestReleaseScheduleSlot tests that releasing a slot correctly decrements count and restores status.
func TestReleaseScheduleSlot(t *testing.T) {
	db := setupTestDB(t)
	sched := seedSchedule(db, 1)

	db.Model(sched).Updates(map[string]interface{}{"current_count": 1, "status": models.ScheduleStatusFull})

	tx := db.Begin()
	s2, err := ReleaseScheduleSlot(tx, sched.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, s2.CurrentCount, "count should go 1 -> 0")
	assert.Equal(t, models.ScheduleStatusAvailable, s2.Status, "status should go full -> available")
	tx.Commit()

	var fresh models.Schedule
	db.First(&fresh, sched.ID)
	assert.Equal(t, 0, fresh.CurrentCount)
	assert.Equal(t, models.ScheduleStatusAvailable, fresh.Status)
}

// TestReleaseScheduleSlotLowerBound tests CurrentCount never goes below 0.
func TestReleaseScheduleSlotLowerBound(t *testing.T) {
	db := setupTestDB(t)
	sched := seedSchedule(db, 10)

	tx := db.Begin()
	s2, err := ReleaseScheduleSlot(tx, sched.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, s2.CurrentCount, "releasing empty slot should not make count negative")
	tx.Commit()
}

// TestTransitionAppointmentStatus tests the core state machine.
func TestTransitionAppointmentStatus(t *testing.T) {
	db := setupTestDB(t)
	p := seedPatient(db)
	s := seedSchedule(db, 10)
	a := seedAppointment(db, p.ID, s.ID, models.AppointmentStatusPending)

	t.Run("pending->cancelled allowed", func(t *testing.T) {
		tx := db.Begin()
		r, changed, err := TransitionAppointmentStatus(
			tx, a.ID,
			[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
			models.AppointmentStatusCancelled,
			"",
		)
		assert.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, models.AppointmentStatusCancelled, r.Status)
		tx.Commit()
	})

	t.Run("cancelled->cancelled idempotent", func(t *testing.T) {
		tx := db.Begin()
		r, changed, err := TransitionAppointmentStatus(
			tx, a.ID,
			[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
			models.AppointmentStatusCancelled,
			"",
		)
		assert.NoError(t, err)
		assert.False(t, changed, "already cancelled should not change")
		assert.Equal(t, models.AppointmentStatusCancelled, r.Status)
		tx.Rollback()
	})

	t.Run("expired->cancelled forbidden", func(t *testing.T) {
		db.Model(a).Update("status", models.AppointmentStatusExpired)
		tx := db.Begin()
		_, _, err := TransitionAppointmentStatus(
			tx, a.ID,
			[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
			models.AppointmentStatusCancelled,
			"",
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidTransition)
		tx.Rollback()
	})

	t.Run("ownership check works", func(t *testing.T) {
		p2 := seedPatient(db)
		s2 := seedSchedule(db, 10)
		a2 := seedAppointment(db, p2.ID, s2.ID, models.AppointmentStatusPending)

		tx := db.Begin()
		_, _, err := TransitionAppointmentStatus(
			tx, a2.ID,
			[]string{models.AppointmentStatusPending},
			models.AppointmentStatusCancelled,
			"patient_id = ?", 999999,
		)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "should fail when patient_id doesn't match")
		tx.Rollback()
	})
}

// TestCancelFlow simulates the full book -> cancel -> re-book flow.
func TestCancelFlow(t *testing.T) {
	db := setupTestDB(t)
	p1 := seedPatient(db)
	p2 := seedPatient(db)
	sched := seedSchedule(db, 1)

	// Step 1: Patient 1 books
	tx := db.Begin()
	occS, qn, err := OccupyScheduleSlot(tx, sched.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, qn)
	assert.Equal(t, 1, occS.CurrentCount)
	assert.Equal(t, models.ScheduleStatusFull, occS.Status, "max=1, current=1 -> full")

	a1 := &models.Appointment{
		PatientID:    p1.ID,
		ScheduleID:   sched.ID,
		DoctorID:     1,
		AppointmentNo: "A1",
		QueueNumber:  qn,
		Status:       models.AppointmentStatusPending,
	}
	assert.NoError(t, tx.Create(a1).Error)
	tx.Commit()

	// Step 2: Patient 2 tries to book -> should fail
	tx = db.Begin()
	_, _, err = OccupyScheduleSlot(tx, sched.ID)
	assert.Error(t, err, "should be fully booked")
	tx.Rollback()

	// Step 3: Patient 1 cancels
	tx = db.Begin()
	appt1, changed, err := TransitionAppointmentStatus(
		tx, a1.ID,
		[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
		models.AppointmentStatusCancelled,
		"patient_id = ?", p1.ID,
	)
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, models.AppointmentStatusCancelled, appt1.Status)

	// THE KEY CHECK: release slot
	_, err = ReleaseScheduleSlot(tx, appt1.ScheduleID)
	assert.NoError(t, err)
	tx.Commit()

	// Verify DB state
	var sFinal models.Schedule
	db.First(&sFinal, sched.ID)
	assert.Equal(t, 0, sFinal.CurrentCount, "BUG WOULD SHOW: count should be 0 after cancel")
	assert.Equal(t, models.ScheduleStatusAvailable, sFinal.Status, "BUG WOULD SHOW: status should be available after cancel")

	// Step 4: Patient 2 now can successfully book the same slot
	tx = db.Begin()
	s3, qn2, err := OccupyScheduleSlot(tx, sched.ID)
	assert.NoError(t, err, "Patient 2 should be able to book released slot")
	assert.Equal(t, 1, qn2)
	assert.Equal(t, 1, s3.CurrentCount)

	a2 := &models.Appointment{
		PatientID:    p2.ID,
		ScheduleID:   sched.ID,
		DoctorID:     1,
		AppointmentNo: "A2",
		QueueNumber:  qn2,
		Status:       models.AppointmentStatusPending,
	}
	assert.NoError(t, tx.Create(a2).Error)
	tx.Commit()

	// Final state: a1 cancelled, a2 pending
	var f1, f2 models.Appointment
	db.First(&f1, a1.ID)
	db.First(&f2, a2.ID)
	assert.Equal(t, models.AppointmentStatusCancelled, f1.Status)
	assert.Equal(t, models.AppointmentStatusPending, f2.Status)
}

// TestCancelFlowWithStatusCheck verifies the exact fix: release based on final
// status == cancelled, not on `changed` flag (which could be fragile).
func TestCancelFlowWithStatusCheck(t *testing.T) {
	db := setupTestDB(t)
	p := seedPatient(db)
	sched := seedSchedule(db, 1)

	tx := db.Begin()
	OccupyScheduleSlot(tx, sched.ID)
	a := &models.Appointment{
		PatientID:    p.ID,
		ScheduleID:   sched.ID,
		DoctorID:     1,
		AppointmentNo: "A",
		QueueNumber:  1,
		Status:       models.AppointmentStatusPending,
	}
	tx.Create(a)
	tx.Commit()

	// Simulate CancelAppointment core logic using the FIXED condition:
	// "if appt.Status == Cancelled && ScheduleID > 0" rather than "if changed"
	tx = db.Begin()
	appt, changed, err := TransitionAppointmentStatus(
		tx, a.ID,
		[]string{models.AppointmentStatusPending, models.AppointmentStatusConfirmed},
		models.AppointmentStatusCancelled,
		"patient_id = ?", p.ID,
	)
	assert.NoError(t, err)
	assert.True(t, changed)

	if appt.Status == models.AppointmentStatusCancelled && appt.ScheduleID > 0 {
		_, err = ReleaseScheduleSlot(tx, appt.ScheduleID)
		assert.NoError(t, err)
	}
	tx.Commit()

	var sFinal models.Schedule
	db.First(&sFinal, sched.ID)
	assert.Equal(t, 0, sFinal.CurrentCount)
	assert.Equal(t, models.ScheduleStatusAvailable, sFinal.Status)
}
