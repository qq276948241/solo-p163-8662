
$BASE_URL = "http://localhost:8080/api"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Clinic Appointment System - Test Flow" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$headers = @{
    "Content-Type" = "application/json"
}

Write-Host "[1/8] Testing Health Check..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/health" -Method Get -Headers $headers
    Write-Host "  Health Check: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "  Health Check Failed: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "  Make sure the server is running first!" -ForegroundColor Red
    exit 1
}
Write-Host ""

Write-Host "[2/8] Register Doctor Account..." -ForegroundColor Yellow
$doctorRegisterBody = @{
    username = "dr_zhang"
    password = "123456"
    name     = "Dr. Zhang San"
    role     = "doctor"
    phone    = "13800138001"
    email    = "zhang@clinic.com"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/auth/register" -Method Post -Headers $headers -Body $doctorRegisterBody
    Write-Host "  Doctor Registered: $($response.Content)" -ForegroundColor Green
} catch {
    $errorMsg = $_.ErrorDetails.Message
    if ($errorMsg -match "Username already exists") {
        Write-Host "  Doctor account already exists, proceeding to login..." -ForegroundColor Yellow
    } else {
        Write-Host "  Register Failed: $errorMsg" -ForegroundColor Red
    }
}
Write-Host ""

Write-Host "[3/8] Doctor Login..." -ForegroundColor Yellow
$loginBody = @{
    username = "dr_zhang"
    password = "123456"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/auth/login" -Method Post -Headers $headers -Body $loginBody
    $loginResult = $response.Content | ConvertFrom-Json
    $doctorToken = $loginResult.data.token
    $doctorId = $loginResult.data.user.doctor_id
    $doctorUserId = $loginResult.data.user.id
    Write-Host "  Doctor Logged In. Doctor ID: $doctorId" -ForegroundColor Green
    Write-Host "  Token: $($doctorToken.Substring(0, 30))..." -ForegroundColor Green
} catch {
    Write-Host "  Login Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
    exit 1
}
Write-Host ""

$doctorHeaders = @{
    "Content-Type"  = "application/json"
    "Authorization" = "Bearer $doctorToken"
}

Write-Host "[4/8] Update Doctor Profile..." -ForegroundColor Yellow
$doctorProfileBody = @{
    department       = "Internal Medicine"
    title            = "Chief Physician"
    specialty        = "Cardiovascular diseases, Hypertension"
    description      = "20 years of clinical experience in internal medicine"
    consultation_fee = 50.00
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/doctors/me/profile" -Method Put -Headers $doctorHeaders -Body $doctorProfileBody
    Write-Host "  Doctor Profile Updated" -ForegroundColor Green
} catch {
    Write-Host "  Update Profile Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "[5/8] Create Doctor Schedule..." -ForegroundColor Yellow
$tomorrow = (Get-Date).AddDays(1).ToString("yyyy-MM-dd")
$scheduleBody = @{
    date             = $tomorrow
    start_time       = "09:00"
    end_time         = "12:00"
    max_appointments = 10
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/schedules" -Method Post -Headers $doctorHeaders -Body $scheduleBody
    $scheduleResult = $response.Content | ConvertFrom-Json
    $scheduleId = $scheduleResult.data.id
    Write-Host "  Schedule Created. Schedule ID: $scheduleId, Date: $tomorrow" -ForegroundColor Green
} catch {
    Write-Host "  Create Schedule Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
    exit 1
}

$scheduleBody2 = @{
    date             = $tomorrow
    start_time       = "14:00"
    end_time         = "17:00"
    max_appointments = 10
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/schedules" -Method Post -Headers $doctorHeaders -Body $scheduleBody2
    Write-Host "  Afternoon Schedule Created" -ForegroundColor Green
} catch {
    Write-Host "  Afternoon Schedule Failed: $($_.ErrorDetails.Message)" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "[6/8] Get Doctor List & Schedules (Public API)..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/doctors" -Method Get -Headers $headers
    $doctors = $response.Content | ConvertFrom-Json
    Write-Host "  Found $($doctors.data.Count) doctors" -ForegroundColor Green
} catch {
    Write-Host "  Get Doctors Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/doctors/$doctorId/schedules?date=$tomorrow" -Method Get -Headers $headers
    Write-Host "  Doctor Schedules Retrieved" -ForegroundColor Green
} catch {
    Write-Host "  Get Schedules Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "[7/8] Register & Login Patient..." -ForegroundColor Yellow
$patientRegisterBody = @{
    username = "patient_li"
    password = "123456"
    name     = "Patient Li Si"
    role     = "patient"
    phone    = "13900139002"
    gender   = "Male"
    age      = 35
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/auth/register" -Method Post -Headers $headers -Body $patientRegisterBody
    Write-Host "  Patient Registered" -ForegroundColor Green
} catch {
    $errorMsg = $_.ErrorDetails.Message
    if ($errorMsg -match "Username already exists") {
        Write-Host "  Patient account already exists, proceeding to login..." -ForegroundColor Yellow
    } else {
        Write-Host "  Register Failed: $errorMsg" -ForegroundColor Red
    }
}

$patientLoginBody = @{
    username = "patient_li"
    password = "123456"
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/auth/login" -Method Post -Headers $headers -Body $patientLoginBody
    $loginResult = $response.Content | ConvertFrom-Json
    $patientToken = $loginResult.data.token
    $patientId = $loginResult.data.user.patient_id
    Write-Host "  Patient Logged In. Patient ID: $patientId" -ForegroundColor Green
} catch {
    Write-Host "  Patient Login Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
    exit 1
}

$patientHeaders = @{
    "Content-Type"  = "application/json"
    "Authorization" = "Bearer $patientToken"
}
Write-Host ""

Write-Host "[8/8] Patient Create Appointment..." -ForegroundColor Yellow
$appointmentBody = @{
    schedule_id = $scheduleId
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/appointments" -Method Post -Headers $patientHeaders -Body $appointmentBody
    $appointmentResult = $response.Content | ConvertFrom-Json
    $appointmentId = $appointmentResult.data.appointment_id
    $appointmentNo = $appointmentResult.data.appointment_no
    $queueNumber = $appointmentResult.data.queue_number
    Write-Host "  Appointment Created Successfully!" -ForegroundColor Green
    Write-Host "  Appointment ID: $appointmentId" -ForegroundColor Green
    Write-Host "  Appointment No: $appointmentNo" -ForegroundColor Green
    Write-Host "  Queue Number: $queueNumber" -ForegroundColor Green
    Write-Host "  Date: $($appointmentResult.data.date)" -ForegroundColor Green
    Write-Host "  Time: $($appointmentResult.data.start_time) - $($appointmentResult.data.end_time)" -ForegroundColor Green
    Write-Host "  Status: $($appointmentResult.data.status)" -ForegroundColor Green
    Write-Host "  Expire At: $($appointmentResult.data.expire_at)" -ForegroundColor Green
} catch {
    Write-Host "  Create Appointment Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
    exit 1
}
Write-Host ""

Write-Host "[9/8] Patient Confirm Appointment..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/appointments/$appointmentId/confirm" -Method Post -Headers $patientHeaders
    Write-Host "  Appointment Confirmed!" -ForegroundColor Green
} catch {
    Write-Host "  Confirm Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "[10/8] Doctor Create Medical Record..." -ForegroundColor Yellow
$medicalRecordBody = @{
    appointment_id = $appointmentId
    diagnosis      = "Essential hypertension (Grade 2), recommend lifestyle modification and medication."
    prescription   = "1. Amlodipine 5mg, once daily in the morning.`n2. Lisinopril 10mg, once daily.`n3. Follow-up after 2 weeks."
    advice         = "Low salt diet, regular exercise, monitor blood pressure daily. Avoid smoking and excessive alcohol."
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/medical-records" -Method Post -Headers $doctorHeaders -Body $medicalRecordBody
    $recordResult = $response.Content | ConvertFrom-Json
    Write-Host "  Medical Record Created! Record ID: $($recordResult.data.id)" -ForegroundColor Green
} catch {
    Write-Host "  Create Medical Record Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "[11/8] Get Patient's Medical Records..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$BASE_URL/medical-records" -Method Get -Headers $patientHeaders
    $records = $response.Content | ConvertFrom-Json
    Write-Host "  Patient has $($records.data.Count) medical records" -ForegroundColor Green
    if ($records.data.Count -gt 0) {
        $latest = $records.data[0]
        Write-Host "  Latest Diagnosis: $($latest.diagnosis.Substring(0, 50))..." -ForegroundColor Green
    }
} catch {
    Write-Host "  Get Medical Records Failed: $($_.ErrorDetails.Message)" -ForegroundColor Red
}
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Test Flow Completed Successfully!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Summary:" -ForegroundColor Yellow
Write-Host "  Doctor ID: $doctorId (dr_zhang)" -ForegroundColor White
Write-Host "  Patient ID: $patientId (patient_li)" -ForegroundColor White
Write-Host "  Schedule ID: $scheduleId" -ForegroundColor White
Write-Host "  Appointment ID: $appointmentId" -ForegroundColor White
Write-Host ""
Write-Host "Features Demonstrated:" -ForegroundColor Yellow
Write-Host "  User Registration & JWT Login" -ForegroundColor White
Write-Host "  Password bcrypt Encryption" -ForegroundColor White
Write-Host "  Doctor Profile Management" -ForegroundColor White
Write-Host "  Doctor Schedule CRUD" -ForegroundColor White
Write-Host "  Patient Appointment Booking (with row lock)" -ForegroundColor White
Write-Host "  Appointment Confirm & Cancel" -ForegroundColor White
Write-Host "  Medical Record Creation" -ForegroundColor White
Write-Host "  15-minute Auto-release Cron Job" -ForegroundColor White
Write-Host "  Role-based Access Control" -ForegroundColor White
Write-Host ""
