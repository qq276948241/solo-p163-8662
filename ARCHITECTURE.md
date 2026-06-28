# 社区诊所预约挂号系统 - 架构文档

> 给接手同事的快速上手指南。Go + Gin + GORM + MySQL 实现的轻量级诊所预约后端。

---

## 一、项目是做什么的

社区诊所给医生排班，患者选医生和时段下单预约挂号，医生完成就诊后录入诊断和处方。核心三件事：

1. **医生排班**：医生维护自己的出诊时间段（上午/下午，设置最多接多少号）
2. **患者预约**：患者看到医生的排班，选时段下单 → 15 分钟内确认，否则自动释放号源；也可手动取消
3. **就诊记录**：医生给预约患者完成就诊，写诊断、处方、医嘱

所有接口 RESTful + JSON 返回，JWT 登录鉴权，密码 bcrypt 加密。

---

## 二、完整业务流程（数据怎么流转的）

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ 1. 注册医生  │────▶│ 2. 设置排班   │────▶│ 3. 患者下单    │────▶│ 4. 患者确认/取消  │────▶│ 5. 医生写就诊记录 │
│             │     │              │     │              │     │  或 15min 自动过期 │     │                  │
│ users+doctors│     │  schedules   │     │ appointments │     │ schedule.Current  │     │  medical_records  │
│  两张表      │     │  Current=0   │     │ status=pending│     │     Count 释放    │     │  appointment_id  │
└─────────────┘     └──────────────┘     └──────┬───────┘     └──────────────────┘     │ appointment.status│
                                                │                                     │   → completed      │
                                                │                                     └──────────────────┘
                                                ▼
                                   ┌────────────────────────┐
                                   │ 3a. 并发防超卖          │
                                   │ SELECT ... FOR UPDATE  │
                                   │ 行锁 schedule 再写入    │
                                   └────────────────────────┘
```

### 每个环节的细节

| 步骤 | 参与者 | 做了什么 | 写了哪些表 |
|---|---|---|---|
| 1. 注册医生 | 管理员/医生自己 | 调 `/api/auth/register`，`role=doctor` | `users`（账号）+ `doctors`（医生资料，外键 user_id） |
| 2. 设置排班 | 医生登录后 | 调 `POST /api/schedules`，传日期、起止时间、最大号数 | `schedules`（doctor_id 外键关联医生），`current_count=0`，`status=available` |
| 3. 患者下单 | 患者登录后 | 先查 `GET /api/doctors/:id/schedules` 看可约时段，再 `POST /api/appointments` | `appointments`，状态 `pending`；同时 schedule 的 `current_count+1`，如果满了 schedule 的 `status=full`。**用事务 + SELECT FOR UPDATE 行锁防超卖** |
| 4. 确认/取消/过期 | 患者或 cron 后台 | `POST /appointments/:id/confirm` 状态→`confirmed`；`POST /appointments/:id/cancel` 状态→`cancelled` **并释放号源**；cron 每 60s 扫一次，超过 `expire_at` 的 pending 订单状态→`expired` 并释放号源 | `appointments.status` + `schedules.current_count` + `schedules.status` |
| 5. 写就诊记录 | 医生登录后 | `POST /api/medical-records`，传 appointment_id、诊断、处方、医嘱 | `medical_records`；同时 appointment 状态→`completed`（号源不释放，就诊已完成） |

---

## 三、目录结构

```
project163/
├── cmd/server/main.go              # 程序入口：加载配置 → 连 DB → 启动 cron → 注册路由 → 跑 Gin
├── internal/
│   ├── config/config.go            # 读 .env，映射到 AppConfig 结构体
│   ├── models/models.go            # 6 张表的 GORM 模型 + 状态/角色常量
│   ├── database/database.go        # InitDB() 连 MySQL + AutoMigrate 自动建表
│   ├── middleware/auth.go          # AuthMiddleware（JWT 校验）+ RoleMiddleware（角色鉴权）
│   ├── utils/
│   │   ├── jwt.go                  # GenerateToken / ParseToken（golang-jwt v5）
│   │   └── password.go             # HashPassword / VerifyPassword（bcrypt）
│   ├── handlers/                   # 业务 HTTP Handler（核心代码在这里）
│   │   ├── auth.go                 # 注册 / 登录 / 当前用户信息
│   │   ├── doctor.go               # 医生列表 / 详情 / 个人资料 CRUD
│   │   ├── schedule.go             # 排班 CRUD（医生视角）+ 查某医生可约排班（公开）
│   │   ├── appointment.go          # ⭐ 预约核心：下单/确认/取消/查询 + 状态机函数（见下）
│   │   ├── medical_record.go       # 就诊记录：医生写 / 患者/医生查
│   │   └── appointment_test.go     # 状态机核心逻辑的单元测试
│   └── cron/cron.go                # 后台 goroutine：每 60s 扫过期预约，调用 handlers.ExpireAppointment 释放号源
├── pkg/response/response.go        # 统一响应格式：{code, message, data}
├── .env                            # 环境变量（DB 连接、JWT 密钥、端口等）
├── go.mod / go.sum
├── test_flow.ps1                   # PowerShell 端到端测试脚本：从注册一路跑到就诊记录
├── test_cancel_rebook.ps1          # PowerShell 专项测试：下单 → 取消 → 另一患者重新下单同个时段
└── README.md / ARCHITECTURE.md     # 你正在看的
```

---

## 四、核心模块怎么配合的

### 4.1 请求处理链路

```
HTTP Request
    │
    ▼
Gin Router（main.go 里注册）
    │
    ├─ 公开路由（/auth/register、/auth/login、/doctors、/health）→ 直接进 Handler
    │
    └─ 需要登录的路由 → AuthMiddleware()
            │  从 Header 拿 Bearer token，ParseToken 解出 userID/role/name 塞进 gin.Context
            ▼
         RoleMiddleware(doctor/patient/admin)  ← 可选，看路由要不要角色校验
            │
            ▼
         Handler（handlers/*.go）
            │  读参数 → 开事务 → 调 models（GORM） → 提交 → response.Success/Error
            ▼
         GORM → models → MySQL
```

### 4.2 handlers ↔ models

handlers 层不直接写 SQL，全部用 GORM 操作 models 里的结构体。每个 handler 基本是：

```go
// 1. 从 c.Get("userID") / c.Param("id") / c.ShouldBindJSON 取参数
// 2. database.DB.Begin() 开事务（涉及多表操作时）
// 3. 事务内调 models 的 CRUD，出错就 tx.Rollback()
// 4. tx.Commit()
// 5. response.Success(c, data) 或 response.BadRequest(c, "msg")
```

### 4.3 ⭐ 预约状态机（重点，集中在 appointment.go）

所有关于预约状态变化的逻辑**只在 [appointment.go](internal/handlers/appointment.go) 里**，不要在别的文件直接改 `appointment.Status` 或 `schedule.current_count`。

```
状态转换总览：
  (创建)  nil             → pending      CreateAppointment  → OccupyScheduleSlot + Transition
  (确认)  pending         → confirmed    ConfirmAppointment → Transition
  (取消)  pending|confirmed → cancelled  CancelAppointment  → Transition + ReleaseScheduleSlot
  (过期)  pending         → expired      cron                → ExpireAppointment（内部 Transition + Release）
  (完成)  confirmed       → completed    CreateMedicalRecord → CompleteAppointment（内部 Transition）
```

抽出的 5 个公共函数（在 appointment.go 顶部）：

| 函数 | 作用 | 谁调 |
|---|---|---|
| `OccupyScheduleSlot(tx, scheduleID)` | 行锁 schedule → 校验名额 → current_count+1 → 满了改 status=full | CreateAppointment |
| `ReleaseScheduleSlot(tx, scheduleID)` | 行锁 schedule → current_count-1（下限 0 保护）→ 有空位改 status=available | CancelAppointment、cron（通过 ExpireAppointment） |
| `TransitionAppointmentStatus(tx, id, allowedFrom, toStatus, lockWhere, ...)` | **所有状态变更唯一入口**：行锁 appointment → 校验 allowedFrom → 改状态 → 返回 (appt, changed, err)。lockWhere 可选，比如传 `"patient_id = ?"` 校验患者只能操作自己的订单 | 所有需要改状态的地方 |
| `ExpireAppointment(tx, id)` | 组合函数：pending→expired + 释放号源。返回原始状态供 cron 打日志 | cron/cron.go |
| `CompleteAppointment(tx, id, lockWhere, ...)` | 组合函数：confirmed→completed（不释放号源） | medical_record.go |

为什么这样设计：
- **状态变更只有一个入口**，不会散落在 N 个 handler 里各写一遍导致漏逻辑（比如之前 ReleaseScheduleSlot 在 Cancel 和 cron 里重复实现过，边界保护还写得不一样）
- **每次改状态都带行锁**，防并发竞态
- **所有权校验嵌在 Transition 里**（lockWhere 参数），A 患者不会操作到 B 患者的订单

### 4.4 cron 怎么独立运行的

在 [main.go:20](cmd/server/main.go#L20) `cron.StartExpiredAppointmentCleaner()` 启动一个后台 goroutine：

```go
// cron/cron.go
func StartExpiredAppointmentCleaner() {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            // 1. 查所有 status=pending 且 expire_at < now 的 appointment
            // 2. 逐个开事务调 handlers.ExpireAppointment(tx, id)
            // 3. 提交事务，打日志
        }
    }()
}
```

关键点：**cron 不自己实现状态改和号源释放**，全部调 handlers 导出的 `ExpireAppointment`，保证和 CancelAppointment 走同一条代码路径，不会漏逻辑。

---

## 五、数据表关系（文字 ER 图）

```
users（账号表，所有角色共用）
  ├─ id (PK)
  ├─ username, password(bcrypt), name, role(doctor/patient/admin), phone, email, gender, age
  │
  ├─ 1:1 → doctors.user_id    （医生如果是 doctor 角色，有对应的医生资料行）
  └─ 1:1 → patients.user_id   （患者如果是 patient 角色，有对应的患者资料行）

doctors（医生资料表）
  ├─ id (PK), user_id (FK → users.id, UNIQUE)
  ├─ department, title, specialty, description, consultation_fee
  └─ 1:N → schedules.doctor_id  （一个医生多个排班）

patients（患者资料表）
  ├─ id (PK), user_id (FK → users.id, UNIQUE)
  ├─ id_card(UNIQUE), address, medical_history
  └─ 1:N → appointments.patient_id   （一个患者多个预约）
      └─ 1:N → medical_records.patient_id （一个患者多个就诊记录）

schedules（排班表）
  ├─ id (PK), doctor_id (FK → doctors.id)
  ├─ date, start_time, end_time
  ├─ max_appointments（最大号数，默认 10）
  ├─ current_count（当前已占用数，默认 0）
  ├─ status（available/full，current_count>=max 时变 full）
  └─ 1:N → appointments.schedule_id  （一个排班时段多个预约号）

appointments（预约表，核心业务表）
  ├─ id (PK)
  ├─ patient_id (FK → patients.id)     下单的患者
  ├─ schedule_id (FK → schedules.id)   预约的时段
  ├─ doctor_id (FK → doctors.id)       冗余字段，查的时候少 JOIN
  ├─ status（pending/confirmed/cancelled/completed/expired）
  ├─ appointment_no (UNIQUE)           预约号，比如 A20240101-0001
  ├─ queue_number                       排队号，1~max_appointments
  ├─ expire_at                          下单时间+15 分钟，超过就被 cron 扫到过期
  └─ 1:1 → medical_records.appointment_id  （一个预约最多一条就诊记录）

medical_records（就诊记录表）
  ├─ id (PK)
  ├─ appointment_id (FK → appointments.id, UNIQUE)
  ├─ patient_id (FK → patients.id)   冗余
  ├─ doctor_id (FK → doctors.id)     冗余
  ├─ diagnosis (诊断，必填), prescription (处方), advice (医嘱)
```

记忆口诀：**users 是根，分支出 doctors 和 patients；doctors 下面挂 schedules；patients 和 schedules 之间通过 appointments 多对多；appointments 被完成后挂 medical_records。**

---

## 六、接口清单

统一前缀：`/api`，统一返回格式：

```json
{ "code": 0, "message": "ok", "data": {...} }
```

`code=0` 表示成功，非 0 表示失败。鉴权接口请带 Header：`Authorization: Bearer <token>`。

### 6.1 认证 Auth（公开）

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| POST | `/auth/register` | 注册。body: `{username, password, name, role(doctor/patient/admin), phone?, email?, gender?, age?}`。role=doctor 会自动建 doctors 行，role=patient 自动建 patients 行 | 否 | - |
| POST | `/auth/login` | 登录。body: `{username, password}`。返回 JWT token + 用户信息 | 否 | - |
| GET | `/auth/me` | 取当前登录用户信息 | 是 | 任意 |

### 6.2 医生 Doctor

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| GET | `/doctors` | 医生列表（公开，患者用来选医生） | 否 | - |
| GET | `/doctors/:id` | 医生详情 | 否 | - |
| GET | `/doctors/:id/schedules?date=YYYY-MM-DD` | 查某医生某一天**可预约**的排班（只返回 status=available 的） | 否 | - |
| GET | `/doctors/me/profile` | 医生看自己的资料 | 是 | doctor / admin |
| PUT | `/doctors/me/profile` | 医生更新自己的资料（科室、职称、专长、诊费等） | 是 | doctor / admin |

### 6.3 排班 Schedule（医生专属）

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| POST | `/schedules` | 新增排班。body: `{date, start_time, end_time, max_appointments?}` | 是 | doctor / admin |
| GET | `/schedules` | 当前登录医生查看自己的所有排班 | 是 | doctor / admin |
| PUT | `/schedules/:id` | 修改排班信息（时间、最大号数等） | 是 | doctor / admin |
| DELETE | `/schedules/:id` | 删除排班（仅当 current_count=0 才能删） | 是 | doctor / admin |

### 6.4 预约 Appointment

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| POST | `/appointments` | 患者下单。body: `{schedule_id}`。事务内行锁 schedule，占用号源；返回排队号和 expire_at | 是 | patient / admin |
| GET | `/appointments` | 查看自己的预约列表（患者看自己的，医生看自己的，admin 看全部） | 是 | 任意 |
| GET | `/appointments/:id` | 查看单条预约详情（要校验是自己的） | 是 | 任意 |
| POST | `/appointments/:id/confirm` | 确认预约（15 分钟内点）。status pending→confirmed | 是 | patient / admin |
| POST | `/appointments/:id/cancel` | 取消预约。status→cancelled，号源释放（幂等，重复调不报错） | 是 | patient / admin |

### 6.5 就诊记录 Medical Record

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| POST | `/medical-records` | 医生写就诊记录。body: `{appointment_id, diagnosis, prescription?, advice?}`。同时 appointment status→completed | 是 | doctor / admin |
| GET | `/medical-records` | 查自己的就诊记录（患者只看自己的，医生看自己写的，admin 全部） | 是 | 任意 |
| GET | `/medical-records/:id` | 看单条就诊记录详情 | 是 | 任意 |
| GET | `/medical-records/patient/:patient_id` | 查某个患者的历史就诊记录 | 是 | doctor / admin |

### 6.6 健康检查

| 方法 | 路径 | 说明 | 登录 | 权限 |
|---|---|---|---|---|
| GET | `/health` | 返回服务状态 | 否 | - |

---

## 七、本地跑起来

### 7.1 环境要求

- Go 1.26+
- MySQL 5.7+ / 8.0+（需要一个账号能建库建表）

### 7.2 环境变量（.env）

项目根目录已有默认的 `.env`，按需改：

```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=123456
DB_NAME=clinic_appointment      # 数据库不存在会自动创建吗？→ 不会，要手动 CREATE DATABASE，然后 InitDB 会 AutoMigrate 建表
DB_CHARSET=utf8mb4
DB_PARSE_TIME=True
DB_LOC=Local

SERVER_HOST=0.0.0.0
SERVER_PORT=8080

JWT_SECRET=clinic_appointment_secret_key_2024   # 生产环境一定要改
JWT_EXPIRE_HOURS=24

APPOINTMENT_TIMEOUT_MINUTES=15     # 下单后多久没确认自动释放
CRON_INTERVAL_SECONDS=60           # 后台扫描过期预约的间隔
```

### 7.3 启动步骤

```bash
# 1. 确保 MySQL 在跑，并建库（只第一次需要）：
mysql -uroot -p123456 -e "CREATE DATABASE IF NOT EXISTS clinic_appointment DEFAULT CHARSET utf8mb4;"

# 2. 下载依赖
go mod download

# 3. 构建并启动
go build -o bin/server.exe ./cmd/server
./bin/server.exe

# 启动日志里会打印所有路由，看到 "Server starting on 0.0.0.0:8080" 就 OK 了。
# 表会通过 GORM AutoMigrate 自动建，不需要手动执行 SQL。
```

### 7.4 跑完整流程验证

PowerShell 脚本 `test_flow.ps1`：从注册医生 → 建排班 → 注册患者 → 下单 → 确认 → 写就诊记录，一条龙跑完，用来快速确认服务正常。

```powershell
# 另开一个终端，服务在跑的情况下：
.\test_flow.ps1
```

专项测试 `test_cancel_rebook.ps1`：专门测"下单→取消→另一个患者再下单同一个时段"这条路径，验证号源释放逻辑是否正确。

### 7.5 跑单元测试

核心状态机逻辑的单测在 `internal/handlers/appointment_test.go`，用 SQLite（modernc 纯 Go 实现，不需要 CGO）：

```bash
go test ./...
```

覆盖的用例：
- 释放号源是否正确减计数、改状态
- 空号源重复释放会不会负数（下限保护）
- 状态转换：pending→cancelled 允许、cancelled→cancelled 幂等、expired→cancelled 禁止
- 患者只能取消自己的订单（所有权校验）
- 完整端到端：A 下单 → B 抢单失败 → A 取消 → B 下单成功

---

## 八、开发时的小提示

1. **不要在 appointment.go 以外的地方直接改 appointment.Status 或 schedule.CurrentCount**，一律用 OccupyScheduleSlot / ReleaseScheduleSlot / TransitionAppointmentStatus 这些封装好的函数，不然会把状态机搞乱。
2. **加新接口时**：先在 models.go 加模型字段（如果需要）→ handlers 里写函数 → main.go 里注册路由 → 别忘了加 AuthMiddleware / RoleMiddleware。
3. **响应格式统一用 pkg/response**，不要自己手写 `c.JSON`。`Success(c, data)` / `SuccessWithMessage(c, msg, data)` / `BadRequest / NotFound / Unauthorized / Forbidden / InternalServerError` 都已经有了。
4. **数据库迁移直接用 GORM AutoMigrate**，项目启动时自动跑。不要手写 DDL，除非是改字段类型/删字段这种 AutoMigrate 不会做的事（这种情况写单独的迁移脚本）。
5. **cron 的 ExpireAppointment 和 Handler 的 CancelAppointment 最终调的是同一组底层函数**，改状态或号源逻辑只改一处就行。
