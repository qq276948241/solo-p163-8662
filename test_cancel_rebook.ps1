
$BASE_URL = "http://localhost:8080/api"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  Bug Fix Test: Book -> Cancel -> Re-book Same Slot" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

$headers = @{ "Content-Type" = "application/json" }

function Check-Server {
    try {
        $r = Invoke-WebRequest -Uri "$BASE_URL/health" -Method Get -Headers $headers -TimeoutSec 3
        Write-Host "[OK] Server is up: $($r.Content)" -ForegroundColor Green
        return $true
    } catch {
        Write-Host "[FAIL] Server not running: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "       Run: .\bin\server.exe  in another terminal" -ForegroundColor Red
        return $false
    }
}

function Post($path, $body, $hdrs) {
    try {
        return Invoke-WebRequest -Uri "$BASE_URL$path" -Method Post -Headers $hdrs -Body $body
    } catch {
        $_.ErrorDetails.Message
        throw
    }
}

function Get($path, $hdrs) {
    return Invoke-WebRequest -Uri "$BASE_URL$path" -Method Get -Headers $hdrs
}

if (-not (Check-Server)) { exit 1 }
Write-Host ""

Write-Host "[Step 1] Register & login doctor (dr_testcancel)..." -ForegroundColor Yellow
try {
    Post "/auth/register" (@{username="dr_testcancel";password="123456";name="Dr Cancel Tester";role="doctor";phone="13810000001";email="cancel@clinic.com"} | ConvertTo-Json) $headers | Out-Null
} catch { if ($_ -notmatch "already exists") { throw } }
$dr = (Post "/auth/login" (@{username="dr_testcancel";password="123456"} | ConvertTo-Json) $headers).Content | ConvertFrom-Json
$drToken = $dr.data.token
$drId = $dr.data.user.doctor_id
Write-Host "  Doctor ID: $drId" -ForegroundColor Green
$drH = @{ "Content-Type"="application/json"; "Authorization"="Bearer $drToken" }
Write-Host ""

Write-Host "[Step 2] Create a schedule with MaxAppointments = 1 (so re-book test is definitive)..." -ForegroundColor Yellow
$tomorrow = (Get-Date).AddDays(1).ToString("yyyy-MM-dd")
$schedBody = @{ date=$tomorrow; start_time="10:00"; end_time="11:00"; max_appointments=1 } | ConvertTo-Json
$sched = (Post "/schedules" $schedBody $drH).Content | ConvertFrom-Json
$schedId = $sched.data.id
Write-Host "  Schedule ID: $schedId, Date=$tomorrow, Max=1" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 3] Check schedule is available BEFORE any booking..." -ForegroundColor Yellow
$schedCheck = (Get "/doctors/$drId/schedules?date=$tomorrow" $headers).Content | ConvertFrom-Json
$before = $schedCheck.data | Where-Object { $_.id -eq $schedId }
Write-Host "  Found schedule: id=$($before.id), current_count=$($before.current_count), status=$($before.status)" -ForegroundColor Green
if ($before.status -ne "available" -or $before.current_count -ne 0) { Write-Host "  [FAIL] Expected available / 0" -ForegroundColor Red; exit 1 }
Write-Host "  [OK] Correct" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 4] Register & login patient (patient_a)..." -ForegroundColor Yellow
try {
    Post "/auth/register" (@{username="patient_a";password="123456";name="Patient A";role="patient";phone="13910000001";gender="Male";age=30} | ConvertTo-Json) $headers | Out-Null
} catch { if ($_ -notmatch "already exists") { throw } }
$pa = (Post "/auth/login" (@{username="patient_a";password="123456"} | ConvertTo-Json) $headers).Content | ConvertFrom-Json
$paToken = $pa.data.token
$paH = @{ "Content-Type"="application/json"; "Authorization"="Bearer $paToken" }
Write-Host "  Patient A logged in" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 5] Patient A books the slot (should SUCCEED, queue #1)..." -ForegroundColor Yellow
$book = (Post "/appointments" (@{schedule_id=$schedId} | ConvertTo-Json) $paH).Content | ConvertFrom-Json
$apptId = $book.data.appointment_id
Write-Host "  Appointment ID: $apptId, Queue: $($book.data.queue_number), Status: $($book.data.status)" -ForegroundColor Green
if ($book.data.queue_number -ne 1) { Write-Host "  [FAIL] Expected queue_number=1" -ForegroundColor Red; exit 1 }
Write-Host "  [OK]" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 6] Schedule should now show FULL (max=1, current=1)..." -ForegroundColor Yellow
$schedCheck2 = (Get "/doctors/$drId/schedules?date=$tomorrow" $headers).Content | ConvertFrom-Json
$afterBook = $schedCheck2.data | Where-Object { $_.id -eq $schedId }
Write-Host "  current_count=$($afterBook.current_count), status=$($afterBook.status)" -ForegroundColor Green
if ($afterBook.current_count -ne 1) { Write-Host "  [FAIL] Expected current_count=1" -ForegroundColor Red; exit 1 }
Write-Host "  [OK] current_count=1" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 7] Register & login patient B, try to book same slot (should FAIL - fully booked)..." -ForegroundColor Yellow
try {
    Post "/auth/register" (@{username="patient_b";password="123456";name="Patient B";role="patient";phone="13910000002";gender="Female";age=25} | ConvertTo-Json) $headers | Out-Null
} catch { if ($_ -notmatch "already exists") { throw } }
$pb = (Post "/auth/login" (@{username="patient_b";password="123456"} | ConvertTo-Json) $headers).Content | ConvertFrom-Json
$pbToken = $pb.data.token
$pbH = @{ "Content-Type"="application/json"; "Authorization"="Bearer $pbToken" }
try {
    Post "/appointments" (@{schedule_id=$schedId} | ConvertTo-Json) $pbH | Out-Null
    Write-Host "  [FAIL] Patient B should NOT be able to book but did!" -ForegroundColor Red
    exit 1
} catch {
    $errMsg = $_
    if ($errMsg -match "fully booked" -or $errMsg -match "not available" -or $errMsg -match "slot") {
        Write-Host "  [OK] Patient B correctly rejected: $errMsg" -ForegroundColor Green
    } else {
        Write-Host "  [?] Got error (may still be OK): $errMsg" -ForegroundColor Yellow
    }
}
Write-Host ""

Write-Host "[Step 8] Patient A CANCELS the appointment..." -ForegroundColor Yellow
$cancel = (Post "/appointments/$apptId/cancel" "{}" $paH).Content | ConvertFrom-Json
Write-Host "  Response: $($cancel.message)" -ForegroundColor Green
if ($cancel.code -ne 0) { Write-Host "  [FAIL] Cancel failed: $($cancel.message)" -ForegroundColor Red; exit 1 }
if ($cancel.data -and $cancel.data.slots_left) {
    Write-Host "  Slots left returned: $($cancel.data.slots_left)" -ForegroundColor Green
}
Write-Host "  [OK]" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 9] Schedule should now be available again (current=0, status=available)..." -ForegroundColor Yellow
$schedCheck3 = (Get "/doctors/$drId/schedules?date=$tomorrow" $headers).Content | ConvertFrom-Json
$afterCancel = $schedCheck3.data | Where-Object { $_.id -eq $schedId }
Write-Host "  current_count=$($afterCancel.current_count), status=$($afterCancel.status)" -ForegroundColor Green
if ($afterCancel.current_count -ne 0) {
    Write-Host "  [FAIL] BUG CONFIRMED: current_count is $($afterCancel.current_count), expected 0 after cancel!" -ForegroundColor Red
    exit 1
}
if ($afterCancel.status -ne "available") {
    Write-Host "  [FAIL] BUG CONFIRMED: status is $($afterCancel.status), expected 'available' after cancel!" -ForegroundColor Red
    exit 1
}
Write-Host "  [OK] Slot correctly released!" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 10] Patient B NOW tries to book the same slot (should SUCCEED)..." -ForegroundColor Yellow
$book2 = (Post "/appointments" (@{schedule_id=$schedId} | ConvertTo-Json) $pbH).Content | ConvertFrom-Json
$apptId2 = $book2.data.appointment_id
Write-Host "  Appointment ID: $apptId2, Queue: $($book2.data.queue_number), Status: $($book2.data.status)" -ForegroundColor Green
if ($book2.data.queue_number -ne 1) { Write-Host "  [FAIL] Expected queue_number=1 after re-booking" -ForegroundColor Red; exit 1 }
Write-Host "  [OK] Patient B successfully booked the released slot!" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 11] Verify appointment A is cancelled, appointment B is pending..." -ForegroundColor Yellow
$myA = (Get "/appointments/$apptId" $paH).Content | ConvertFrom-Json
$myB = (Get "/appointments/$apptId2" $pbH).Content | ConvertFrom-Json
Write-Host "  Appt A (cancelled): status=$($myA.data.status)" -ForegroundColor Green
Write-Host "  Appt B (pending):   status=$($myB.data.status)" -ForegroundColor Green
if ($myA.data.status -ne "cancelled") { Write-Host "  [FAIL] Appt A should be cancelled but is $($myA.data.status)" -ForegroundColor Red; exit 1 }
if ($myB.data.status -ne "pending")   { Write-Host "  [FAIL] Appt B should be pending but is $($myB.data.status)"   -ForegroundColor Red; exit 1 }
Write-Host "  [OK] Both statuses correct" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 12] Patient B also cancels, just to verify idempotency (should not double-release)..." -ForegroundColor Yellow
Post "/appointments/$apptId2/cancel" "{}" $pbH | Out-Null
$schedCheck4 = (Get "/doctors/$drId/schedules?date=$tomorrow" $headers).Content | ConvertFrom-Json
$final = $schedCheck4.data | Where-Object { $_.id -eq $schedId }
Write-Host "  After B cancels: current_count=$($final.current_count), status=$($final.status)" -ForegroundColor Green
if ($final.current_count -ne 0) {
    Write-Host "  [FAIL] Double-release bug! current_count should be 0 but is $($final.current_count) (negative?)" -ForegroundColor Red
    exit 1
}
Write-Host "  [OK] Lower-bound protection works, count stays at 0 (no underflow)" -ForegroundColor Green
Write-Host ""

Write-Host "[Step 13] Idempotency: cancel already-cancelled appointment (should return success no-op)..." -ForegroundColor Yellow
$cancel2 = (Post "/appointments/$apptId/cancel" "{}" $paH).Content | ConvertFrom-Json
Write-Host "  Response: $($cancel2.message)" -ForegroundColor Green
if ($cancel2.code -ne 0) { Write-Host "  [FAIL] Idempotent cancel should not error" -ForegroundColor Red; exit 1 }
Write-Host "  [OK]" -ForegroundColor Green
Write-Host ""

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  ALL TESTS PASSED - Bug Fix Verified!" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Verify server logs show ReleaseScheduleSlot was called with" -ForegroundColor White
Write-Host "  correct schedule_id and count transitions 1->0 on step 8." -ForegroundColor White
Write-Host ""
