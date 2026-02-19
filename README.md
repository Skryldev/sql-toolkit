<div dir="rtl">

# sqltoolkit — کیت ابزار SQL سطح Production برای Go

<div align="center">

```
SQL-first  ·  Ultra-performant  ·  Developer-friendly  ·  Clean Architecture  ·  Fully Testable
```

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen)]()
[![Race Condition Free](https://img.shields.io/badge/Race--Safe-Yes-brightgreen)]()

</div>

---

## 📋 فهرست مطالب

- [چرا sqltoolkit؟](#-چرا-sqltoolkit)
- [sqltoolkit چیست؟](#-sqltoolkit-چیست)
- [این ابزار ORM نیست](#-این-ابزار-orm-نیست)
- [ویژگی‌های اصلی](#-ویژگیهای-اصلی)
- [ساختار پروژه](#-ساختار-پروژه)
- [نصب و راه‌اندازی](#-نصب-و-راهاندازی)
- [شروع سریع](#-شروع-سریع)
- [راهنمای کامل استفاده](#-راهنمای-کامل-استفاده)
  - [۱. باز کردن اتصال به دیتابیس](#۱-باز-کردن-اتصال-به-دیتابیس)
  - [۲. اجرای Query ها](#۲-اجرای-query-ها)
  - [۳. مدیریت Transaction](#۳-مدیریت-transaction)
  - [۴. الگوی Repository](#۴-الگوی-repository)
  - [۵. مدیریت خطا با Type Safety](#۵-مدیریت-خطا-با-type-safety)
  - [۶. Batch Operations](#۶-batch-operations)
  - [۷. Retry و Timeout](#۷-retry-و-timeout)
  - [۸. سیستم Hook (لاگ / متریک / تریسینگ)](#۸-سیستم-hook-لاگ--متریک--تریسینگ)
  - [۹. Migration](#۹-migration)
  - [۱۰. Driver سفارشی](#۱۰-driver-سفارشی)
- [تست‌نویسی](#-تستنویسی)
- [تصمیمات معماری](#-تصمیمات-معماری)
- [نکات پرفورمنس](#-نکات-پرفورمنس)
- [مقایسه با ابزارهای مشابه](#-مقایسه-با-ابزارهای-مشابه)

---

## 🤔 چرا sqltoolkit؟

اگر با Go و دیتابیس کار کرده‌اید، احتمالاً با این دردسرها آشنا هستید:

**مشکل با raw `database/sql`:**
- کد boilerplate بسیار زیاد
- مدیریت دستی `rows.Close()` و احتمال leak
- هیچ استانداردی برای مدیریت خطا وجود ندارد
- پیاده‌سازی transaction در هر جایی متفاوت است
- اضافه کردن لاگ یا متریک نیاز به دستکاری همه جای کد دارد

**مشکل با ORM ها (GORM, ent, ...):**
- SQL پنهان است — نمی‌دانید دقیقاً چه چیزی اجرا می‌شود
- Performance غیرقابل پیش‌بینی به دلیل query generation خودکار
- N+1 query بدون اینکه متوجه شوید
- وابستگی سنگین و learning curve بالا
- تست کردن دشوار است

**راه‌حل sqltoolkit:**
> یک لایه نازک، سریع، و شفاف روی `database/sql` که boilerplate را حذف می‌کند بدون اینکه کنترل SQL را از شما بگیرد.

---

## 🔍 sqltoolkit چیست؟

sqltoolkit یک **Database Toolkit** است، نه ORM. تفاوت اساسی اینجاست:

| ویژگی | ORM | sqltoolkit |
|---|---|---|
| SQL | پنهان و auto-generate | **همیشه explicit و قابل مشاهده** |
| کنترل query | محدود | **کامل** |
| Performance | غیرقابل پیش‌بینی | **قابل پیش‌بینی و minimal overhead** |
| یادگیری | پیچیده | **ساده — فقط SQL بلد باشید** |
| Debug | سخت | **آسان — دقیقاً می‌دانید چه اجرا می‌شود** |

این ابزار برای توسعه‌دهندگانی طراحی شده که:
- می‌خواهند SQL بنویسند، نه کد Go برای توصیف SQL
- به performance اهمیت می‌دهند
- به testability و clean architecture اهمیت می‌دهند
- نمی‌خواهند به یک ORM وابسته باشند

---

## 🚫 این ابزار ORM نیست

این موارد را **هرگز** در این toolkit پیدا نخواهید کرد:

```go
// ❌ اینها وجود ندارند:
db.Where("name = ?", "Alice").Find(&users)   // auto query generation
db.Preload("Orders").Find(&users)             // implicit join
db.AutoMigrate(&User{})                       // schema از struct
db.Model(&user).Updates(user)                 // hidden UPDATE
```

```go
// ✅ اینجا همه چیز explicit است:
rows, err := db.Query(ctx, `
    SELECT u.id, u.name, o.total
    FROM   users u
    JOIN   orders o ON o.user_id = u.id
    WHERE  u.name = $1
`, "Alice")
```

---

## ✨ ویژگی‌های اصلی

### ۱. SQL-First
تمام query ها explicit هستند. هیچ query ای بدون اطلاع شما اجرا نمی‌شود.

### ۲. Connection Pool هوشمند
پیکربندی کامل pool با timeout، max connections، و graceful shutdown.

### ۳. Transaction Helper ایمن
`ExecTx` به صورت خودکار commit/rollback می‌کند، حتی در صورت panic.

### ۴. مدیریت خطای یکپارچه
خطاهای PostgreSQL، MySQL، و SQLite همه به sentinel error های یکسان map می‌شوند.

### ۵. سیستم Hook pluggable
لاگ، متریک، و تریسینگ بدون تغییر در کد اصلی.

### ۶. الگوی Querier
Repository ها هم با `*DB` و هم با `*Tx` کار می‌کنند — همان کد، بدون تغییر.

### ۷. Driver pluggable
پشتیبانی از PostgreSQL، MySQL، SQLite، و هر driver سفارشی.

### ۸. Batch Operations کارآمد
درج و به‌روزرسانی انبوه با prepared statement و در یک transaction.

### ۹. Retry با backoff
مقاومت در برابر deadlock و timeout با retry قابل پیکربندی.

### ۱۰. Migration یکپارچه
CLI کامل برای مدیریت migration با `golang-migrate`.

---

## 📁 ساختار پروژه

```
sqltoolkit/
│
├── db/                          # هسته اصلی toolkit
│   ├── db.go                    # *DB wrapper، pool، Exec/Query/QueryRow
│   ├── tx.go                    # *Tx wrapper، ExecTx، Querier interface
│   ├── errors.go                # Sentinel errors + ErrorMapper interface
│   ├── hooks.go                 # Hook interface + built-in ها
│   ├── driver.go                # Driver interface + adapters
│   ├── env.go                   # خواندن environment variable
│   ├── context_errors.go        # اتصال context sentinels
│   └── db_test.go               # Unit tests (SQLite in-memory)
│
├── models/                      # Domain models — struct های ساده Go
│   └── user.go
│
├── repo/                        # لایه Data Access با SQL های explicit
│   ├── user_repo.go
│   └── user_repo_test.go
│
├── migrations/                  # فایل‌های SQL برای تغییرات schema
│   ├── 000001_create_users.up.sql
│   └── 000001_create_users.down.sql
│
├── cmd/
│   └── migrate/                 # CLI مستقل برای اجرای migration
│       └── main.go
│
├── main.go                      # نمونه‌های کامل استفاده
├── go.mod
└── README.md
```

**چرا این ساختار؟**

- **`db/`** از هر چیزی خارج از standard library مستقل است — قابل استفاده در هر پروژه‌ای.
- **`models/`** فقط struct است — هیچ وابستگی به db ندارد.
- **`repo/`** تنها لایه‌ای است که SQL می‌نویسد. business logic هرگز مستقیم با db کار نمی‌کند.
- **`cmd/migrate/`** کاملاً جدا از runtime است — اجرای migration هرگز با کد production مخلوط نمی‌شود.

---

## 📦 نصب و راه‌اندازی

### پیش‌نیازها

- Go 1.22 یا بالاتر
- یکی از دیتابیس‌های: PostgreSQL، MySQL، یا SQLite

### نصب

```bash
# اضافه کردن ماژول به پروژه
go get github.com/yourorg/sqltoolkit

# driver موردنظر را نصب کنید:

# PostgreSQL (lib/pq)
go get github.com/lib/pq

# PostgreSQL (pgx — پرفورمنس بالاتر)
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/stdlib

# MySQL
go get github.com/go-sql-driver/mysql

# SQLite (نیاز به CGO دارد)
go get github.com/mattn/go-sqlite3

# Migration
go get github.com/golang-migrate/migrate/v4
```

---

## ⚡ شروع سریع

کمترین کدی که برای شروع کار نیاز دارید:

```go
package main

import (
    "context"
    "log"

    "github.com/yourorg/sqltoolkit/db"
    _ "github.com/lib/pq"
)

func main() {
    // ۱. اتصال به دیتابیس
    database := db.MustOpen(db.Config{
        DSN:        "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
        DriverName: "postgres",
    })
    defer database.Close()

    ctx := context.Background()

    // ۲. اجرای یک query ساده
    var name string
    err := database.QueryRow(ctx,
        `SELECT name FROM users WHERE id = $1`, 1,
    ).Scan(&name)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("نام:", name)
}
```

---

## 📖 راهنمای کامل استفاده

### ۱. باز کردن اتصال به دیتابیس

#### روش اول — پیکربندی مستقیم (توصیه شده)

```go
import (
    "time"
    "github.com/yourorg/sqltoolkit/db"
    _ "github.com/lib/pq" // ثبت PostgreSQL driver
)

database, err := db.Open(db.Config{
    // اتصال
    DSN:        "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
    DriverName: "postgres",

    // تنظیمات Connection Pool
    MaxOpenConns:    25,              // حداکثر اتصال همزمان
    MaxIdleConns:    10,              // اتصال‌های idle در pool
    ConnMaxLifetime: 5 * time.Minute, // طول عمر هر اتصال
    ConnMaxIdleTime: 2 * time.Minute, // مدت idle قبل از بسته شدن

    // timeout پیش‌فرض — اگر context ای deadline نداشته باشد
    DefaultTimeout: 10 * time.Second,
})
if err != nil {
    log.Fatalf("اتصال به دیتابیس ناموفق: %v", err)
}
defer database.Close()
```

#### روش دوم — استفاده از Environment Variable

```go
// DATABASE_URL به صورت خودکار خوانده می‌شود
dsn, err := db.DSNFromEnv()
if err != nil {
    log.Fatal(err)
}

database, err := db.Open(db.Config{
    DSN:        dsn,
    DriverName: "postgres",
})
```

#### روش سوم — MustOpen (برای main.go)

```go
// در صورت خطا panic می‌کند — مناسب برای init اپلیکیشن
database := db.MustOpen(db.Config{
    DSN:        os.Getenv("DATABASE_URL"),
    DriverName: "postgres",
})
```

#### روش چهارم — OpenWithDriver (ساختارمند)

```go
database, err := db.OpenWithDriver("postgres", db.DriverOptions{
    Host:     "localhost",
    Port:     5432,
    User:     "myuser",
    Password: "mypass",
    Database: "mydb",
    SSLMode:  "disable",
}, db.Config{
    MaxOpenConns: 25,
    DefaultTimeout: 10 * time.Second,
})
```

#### Health Check

```go
// بررسی سلامت اتصال
if err := database.Ping(ctx); err != nil {
    log.Printf("دیتابیس در دسترس نیست: %v", err)
}

// آمار Connection Pool
stats := database.Stats()
log.Printf("اتصال‌های باز: %d، idle: %d، در استفاده: %d",
    stats.OpenConnections, stats.Idle, stats.InUse)
```

---

### ۲. اجرای Query ها

#### Exec — برای INSERT، UPDATE، DELETE، DDL

```go
// INSERT ساده
res, err := database.Exec(ctx,
    `INSERT INTO products (name, price, stock) VALUES ($1, $2, $3)`,
    "لپ‌تاپ", 25000000, 10,
)
if err != nil {
    return err
}

// بررسی تعداد ردیف‌های تأثیرپذیر
affected, _ := res.RowsAffected()
log.Printf("%d ردیف درج شد", affected)

// UPDATE
_, err = database.Exec(ctx,
    `UPDATE products SET stock = stock - $1 WHERE id = $2`,
    1, productID,
)

// DELETE
_, err = database.Exec(ctx,
    `DELETE FROM sessions WHERE expires_at < $1`,
    time.Now(),
)
```

#### QueryRow — یک ردیف

```go
// SELECT یک مقدار
var count int64
err := database.QueryRow(ctx,
    `SELECT COUNT(*) FROM users WHERE active = true`,
).Scan(&count)

// SELECT چند فیلد
var id int64
var name, email string
var createdAt time.Time

err = database.QueryRow(ctx,
    `SELECT id, name, email, created_at FROM users WHERE id = $1`,
    userID,
).Scan(&id, &name, &email, &createdAt)

if db.IsNotFound(err) {
    // کاربر وجود ندارد
    return nil, ErrUserNotFound
}
if err != nil {
    return nil, err
}
```

#### Query — چند ردیف

```go
rows, err := database.Query(ctx, `
    SELECT id, name, email, created_at
    FROM   users
    WHERE  active = true
    ORDER  BY created_at DESC
    LIMIT  $1 OFFSET $2
`, limit, offset)
if err != nil {
    return nil, err
}
defer rows.Close() // ← همیشه Close کنید

var users []User
for rows.Next() {
    var u User
    if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt); err != nil {
        return nil, fmt.Errorf("scan: %w", err)
    }
    users = append(users, u)
}

// بررسی خطاهای iteration
if err := rows.Err(); err != nil {
    return nil, err
}

return users, nil
```

#### Prepare — Prepared Statement (برای query های تکراری)

```go
// ساخت prepared statement
stmt, err := database.Prepare(ctx,
    `SELECT id, name FROM users WHERE email = $1`)
if err != nil {
    return err
}
defer stmt.Close()

// استفاده مکرر بدون re-parse
for _, email := range emails {
    var id int64
    var name string
    err := stmt.QueryRow(ctx, email).Scan(&id, &name)
    // ...
}
```

---

### ۳. مدیریت Transaction

#### ExecTx — ساده‌ترین روش (توصیه شده)

```go
// انتقال موجودی بین دو حساب
err := database.ExecTx(ctx, func(tx *db.Tx) error {
    // کسر از حساب مبدأ
    res, err := tx.Exec(ctx,
        `UPDATE accounts SET balance = balance - $1 WHERE id = $2 AND balance >= $1`,
        amount, fromAccountID,
    )
    if err != nil {
        return err // ← ROLLBACK خودکار
    }
    if n, _ := res.RowsAffected(); n == 0 {
        return errors.New("موجودی کافی نیست") // ← ROLLBACK خودکار
    }

    // افزودن به حساب مقصد
    _, err = tx.Exec(ctx,
        `UPDATE accounts SET balance = balance + $1 WHERE id = $2`,
        amount, toAccountID,
    )
    if err != nil {
        return err // ← ROLLBACK خودکار
    }

    // ثبت لاگ تراکنش
    _, err = tx.Exec(ctx,
        `INSERT INTO transfer_logs (from_id, to_id, amount, created_at) VALUES ($1, $2, $3, $4)`,
        fromAccountID, toAccountID, amount, time.Now(),
    )
    return err // nil → COMMIT ، غیر nil → ROLLBACK

}) // panic نیز باعث ROLLBACK می‌شود
```

#### Transaction با Isolation Level سفارشی

```go
err := database.ExecTx(ctx, func(tx *db.Tx) error {
    // عملیات حساس به race condition
    var stock int
    if err := tx.QueryRow(ctx,
        `SELECT stock FROM products WHERE id = $1 FOR UPDATE`,
        productID,
    ).Scan(&stock); err != nil {
        return err
    }

    if stock < quantity {
        return ErrInsufficientStock
    }

    _, err := tx.Exec(ctx,
        `UPDATE products SET stock = stock - $1 WHERE id = $2`,
        quantity, productID,
    )
    return err

}, db.TxOptions{
    Isolation: sql.LevelSerializable, // سطح isolation
    ReadOnly:  false,
})
```

#### Panic در Transaction

```go
// حتی اگر panic رخ دهد، ROLLBACK انجام می‌شود
err := database.ExecTx(ctx, func(tx *db.Tx) error {
    _, _ = tx.Exec(ctx, `INSERT INTO logs VALUES ($1)`, "شروع")
    panic("اتفاق غیرمنتظره") // ← ROLLBACK انجام می‌شود و panic re-panic می‌شود
    return nil
})
// err == nil اما panic هنوز propagate می‌شود
```

---

### ۴. الگوی Repository

مهم‌ترین قابلیت طراحی: **`db.Querier` interface**.

هم `*DB` و هم `*Tx` این interface را پیاده‌سازی می‌کنند، بنابراین Repository ها در هر دو context کار می‌کنند.

#### تعریف interface

```go
// repo/user_repo.go

type UserRepository interface {
    Insert(ctx context.Context, params CreateUserParams) (*User, error)
    GetByID(ctx context.Context, id int64) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    List(ctx context.Context, limit, offset int) ([]*User, error)
    Update(ctx context.Context, params UpdateUserParams) (*User, error)
    Delete(ctx context.Context, id int64) error
    Count(ctx context.Context) (int64, error)
}

type userRepo struct {
    q db.Querier // ← نه *db.DB — بلکه interface
}

func NewUserRepo(q db.Querier) UserRepository {
    return &userRepo{q: q}
}
```

#### پیاده‌سازی با SQL های explicit

```go
// SQL ها به عنوان ثابت تعریف می‌شوند — کاملاً قابل مشاهده
const sqlGetUserByID = `
    SELECT id, name, email, role, created_at, updated_at
    FROM   users
    WHERE  id = $1
      AND  deleted_at IS NULL
    LIMIT  1`

func (r *userRepo) GetByID(ctx context.Context, id int64) (*User, error) {
    u := &User{}
    err := r.q.QueryRow(ctx, sqlGetUserByID, id).Scan(
        &u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("GetByID: %w", err)
    }
    return u, nil
}
```

#### Update جزئی (Partial Update)

```go
// پارامترهای Update با pointer — فقط فیلدهای غیر nil به‌روز می‌شوند
type UpdateUserParams struct {
    ID    int64
    Name  *string // nil = تغییر نده
    Email *string // nil = تغییر نده
    Role  *string // nil = تغییر نده
}

func (r *userRepo) Update(ctx context.Context, params UpdateUserParams) (*User, error) {
    setClauses := []string{}
    args := []any{}
    i := 1

    if params.Name != nil {
        setClauses = append(setClauses, fmt.Sprintf("name = $%d", i))
        args = append(args, *params.Name)
        i++
    }
    if params.Email != nil {
        setClauses = append(setClauses, fmt.Sprintf("email = $%d", i))
        args = append(args, *params.Email)
        i++
    }
    if params.Role != nil {
        setClauses = append(setClauses, fmt.Sprintf("role = $%d", i))
        args = append(args, *params.Role)
        i++
    }
    if len(setClauses) == 0 {
        return r.GetByID(ctx, params.ID)
    }

    setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", i))
    args = append(args, time.Now().UTC())
    i++

    args = append(args, params.ID)
    query := fmt.Sprintf(`
        UPDATE users
        SET    %s
        WHERE  id = $%d
        RETURNING id, name, email, role, created_at, updated_at`,
        strings.Join(setClauses, ", "), i)

    u := &User{}
    err := r.q.QueryRow(ctx, query, args...).Scan(
        &u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt,
    )
    return u, err
}
```

#### استفاده در Service Layer

```go
// service/user_service.go

type UserService struct {
    db       *db.DB
    userRepo repo.UserRepository
}

// عملیات معمولی
func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    return s.userRepo.GetByID(ctx, id)
}

// عملیات چند مرحله‌ای درون transaction
func (s *UserService) RegisterWithProfile(ctx context.Context, input RegisterInput) error {
    return s.db.ExecTx(ctx, func(tx *db.Tx) error {
        // همان repo، اما اینبار با *Tx به جای *DB
        userRepo := repo.NewUserRepo(tx)
        profileRepo := repo.NewProfileRepo(tx)

        user, err := userRepo.Insert(ctx, repo.CreateUserParams{
            Name:  input.Name,
            Email: input.Email,
        })
        if err != nil {
            return err
        }

        _, err = profileRepo.Insert(ctx, repo.CreateProfileParams{
            UserID: user.ID,
            Bio:    input.Bio,
        })
        return err
        // اگر هر کدام از دو Insert خطا داشته باشند، هر دو rollback می‌شوند
    })
}
```

---

### ۵. مدیریت خطا با Type Safety

#### Sentinel Error ها

```go
// خطاهای آماده که errors.Is() روی آن‌ها کار می‌کند:
db.ErrNotFound           // ردیف پیدا نشد (sql.ErrNoRows)
db.ErrDuplicateKey       // نقض unique constraint
db.ErrForeignKeyViolation // نقض foreign key
db.ErrDeadlock           // deadlock شناسایی شد
db.ErrTimeout            // query از زمان مجاز تجاوز کرد
db.ErrCheckViolation     // نقض CHECK constraint
db.ErrConnectionFailed   // اتصال به دیتابیس ناموفق
```

#### الگوی استفاده در HTTP Handler

```go
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
    id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

    user, err := h.userRepo.GetByID(r.Context(), id)
    switch {
    case db.IsNotFound(err):
        http.Error(w, "کاربر یافت نشد", http.StatusNotFound)
        return
    case err != nil:
        log.Printf("خطای دیتابیس: %v", err)
        http.Error(w, "خطای داخلی سرور", http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
    // ...
    _, err := h.userRepo.Insert(r.Context(), params)
    switch {
    case db.IsDuplicateKey(err):
        http.Error(w, "این ایمیل قبلاً ثبت شده", http.StatusConflict)
    case db.IsForeignKeyViolation(err):
        http.Error(w, "مرجع داده نامعتبر است", http.StatusBadRequest)
    case err != nil:
        http.Error(w, "خطای داخلی سرور", http.StatusInternalServerError)
    default:
        w.WriteHeader(http.StatusCreated)
    }
}
```

#### دسترسی به خطای خام driver

```go
_, err := userRepo.Insert(ctx, params)
if err != nil {
    var dbErr *db.DBError
    if errors.As(err, &dbErr) {
        log.Printf("sentinel: %v", dbErr.Sentinel) // db.ErrDuplicateKey
        log.Printf("driver error: %v", dbErr.Cause) // pq: ERROR: duplicate key...
        log.Printf("message: %s", dbErr.Message)
    }
}
```

#### Mapper سفارشی

```go
// اضافه کردن error code های اختصاصی CockroachDB
type crdbMapper struct{}

func (crdbMapper) Map(err error) error {
    if err == nil {
        return nil
    }
    // CRDB خطای خاص خود را دارد
    if strings.Contains(err.Error(), "restart transaction") {
        return &db.DBError{Sentinel: db.ErrDeadlock, Cause: err}
    }
    return err // به mapper پیش‌فرض pass می‌شود
}

database.SetErrorMapper(db.ChainMapper(crdbMapper{}, db.DefaultErrorMapper()))
```

---

### ۶. Batch Operations

#### BatchExec — generic، کارآمد، در یک transaction

```go
type OrderItem struct {
    ProductID int64
    Quantity  int
    Price     float64
}

items := []OrderItem{
    {ProductID: 1, Quantity: 2, Price: 150000},
    {ProductID: 3, Quantity: 1, Price: 89000},
    {ProductID: 7, Quantity: 5, Price: 25000},
}

// همه ردیف‌ها در یک transaction با یک prepared statement درج می‌شوند
err := db.BatchExec(database, ctx,
    `INSERT INTO order_items (order_id, product_id, quantity, price)
     VALUES ($1, $2, $3, $4)`,
    items,
    func(item OrderItem) []any {
        return []any{orderID, item.ProductID, item.Quantity, item.Price}
    },
)
if err != nil {
    return fmt.Errorf("درج آیتم‌های سفارش ناموفق: %w", err)
}
```

#### Batch با بازگشت نتیجه

```go
// BatchInsert در repo — همه کاربران را با ID برمی‌گرداند
users, err := userRepo.BatchInsert(ctx, []models.CreateUserParams{
    {Name: "علی", Email: "ali@example.com"},
    {Name: "سارا", Email: "sara@example.com"},
    {Name: "رضا", Email: "reza@example.com"},
})
if err != nil {
    return err
}
for _, u := range users {
    log.Printf("درج شد: ID=%d, Email=%s", u.ID, u.Email)
}
```

#### Batch Update با Transaction دستی

```go
err := database.ExecTx(ctx, func(tx *db.Tx) error {
    stmt, err := tx.Prepare(ctx,
        `UPDATE inventory SET quantity = $1, updated_at = $2 WHERE product_id = $3`)
    if err != nil {
        return err
    }
    defer stmt.Close()

    now := time.Now().UTC()
    for _, update := range inventoryUpdates {
        _, err := stmt.Exec(ctx, update.NewQuantity, now, update.ProductID)
        if err != nil {
            return fmt.Errorf("update product %d: %w", update.ProductID, err)
        }
    }
    return nil
})
```

---

### ۷. Retry و Timeout

#### Timeout برای یک عملیات خاص

```go
// context با timeout برای query حساس به زمان
queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
defer cancel()

var result BigReport
err := database.QueryRow(queryCtx,
    `SELECT * FROM generate_heavy_report($1)`, reportID,
).Scan(&result)

if db.IsTimeout(err) {
    log.Println("گزارش خیلی زمان برد — بعداً تلاش کنید")
}
```

#### WithRetry — retry هوشمند

```go
// retry فقط برای خطاهای قابل retry
err := db.WithRetry(ctx, db.RetryConfig{
    MaxAttempts: 5,
    Delay:       50 * time.Millisecond,
    RetryOn: func(err error) bool {
        return db.IsDeadlock(err) || db.IsTimeout(err)
    },
}, func() error {
    return database.ExecTx(ctx, func(tx *db.Tx) error {
        // عملیاتی که ممکن است deadlock رخ دهد
        _, err := tx.Exec(ctx,
            `UPDATE counters SET value = value + 1 WHERE id = $1`,
            counterID,
        )
        return err
    })
})

if err != nil {
    log.Printf("بعد از ۵ تلاش ناموفق: %v", err)
}
```

#### DefaultTimeout در سطح Config

```go
// همه query هایی که context بدون deadline دارند
// به صورت خودکار timeout می‌شوند
database := db.MustOpen(db.Config{
    DSN:            dsn,
    DriverName:     "postgres",
    DefaultTimeout: 30 * time.Second, // ← timeout پیش‌فرض
})

// این query اگر بیشتر از ۳۰ ثانیه طول بکشد، خودکار cancel می‌شود
rows, err := database.Query(context.Background(), `SELECT * FROM large_table`)

// اگر context قبلاً deadline داشته باشد، DefaultTimeout نادیده گرفته می‌شود
ctx5s, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
rows, err = database.Query(ctx5s, `SELECT * FROM large_table`) // timeout: 5s
```

---

### ۸. سیستم Hook (لاگ / متریک / تریسینگ)

#### LogHook — آماده استفاده

```go
db.NewLogHook(db.LogHookConfig{
    Logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    })),
    SlowQueryThreshold: 200 * time.Millisecond, // ← query های کند هشدار می‌دهند
    LogArgs:            false,                   // ← در production false بگذارید (PII)
})
```

نمونه خروجی لاگ:
```json
{"level":"DEBUG","msg":"sqltoolkit/db: query","query":"SELECT id, name FROM users WHERE id = $1","duration":"1.2ms"}
{"level":"WARN","msg":"sqltoolkit/db: slow query","query":"SELECT * FROM reports WHERE...","duration":"350ms"}
{"level":"ERROR","msg":"sqltoolkit/db: query error","query":"INSERT INTO...","error":"duplicate key value"}
```

#### MetricsHook — با Prometheus

```go
// پیاده‌سازی MetricsCollector برای Prometheus
type prometheusCollector struct {
    queryDuration *prometheus.HistogramVec
    queryErrors   *prometheus.CounterVec
}

func (p *prometheusCollector) RecordQuery(query string, d time.Duration, success bool) {
    // نام operation را از query استخراج کنید
    op := extractOperation(query) // "SELECT", "INSERT", "UPDATE", "DELETE"

    p.queryDuration.WithLabelValues(op).Observe(d.Seconds())
    if !success {
        p.queryErrors.WithLabelValues(op).Inc()
    }
}

// ثبت در Config
database := db.MustOpen(db.Config{
    // ...
    Hooks: []db.Hook{
        db.NewMetricsHook(&prometheusCollector{
            queryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
                Name:    "db_query_duration_seconds",
                Help:    "مدت زمان اجرای query های دیتابیس",
                Buckets: prometheus.DefBuckets,
            }, []string{"operation"}),
            queryErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
                Name: "db_query_errors_total",
                Help: "تعداد خطاهای query دیتابیس",
            }, []string{"operation"}),
        }),
    },
})
```

#### TracingHook — با OpenTelemetry

```go
// پیاده‌سازی Tracer برای OpenTelemetry
type otelTracer struct {
    tracer trace.Tracer
}

func (t *otelTracer) StartSpan(ctx context.Context, query string) context.Context {
    ctx, _ = t.tracer.Start(ctx, "db.query",
        trace.WithAttributes(
            attribute.String("db.statement", query),
            attribute.String("db.system", "postgresql"),
        ),
    )
    return ctx
}

func (t *otelTracer) EndSpan(ctx context.Context, err error) {
    span := trace.SpanFromContext(ctx)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    }
    span.End()
}

// استفاده
db.NewTracingHook(&otelTracer{
    tracer: otel.Tracer("sqltoolkit"),
})
```

#### Hook سفارشی

```go
// می‌توانید هر Hook دلخواهی بنویسید
type auditHook struct {
    logger *slog.Logger
}

func (h *auditHook) BeforeQuery(ctx context.Context, query string, args []any) {
    // قبل از اجرا — می‌توانید request ID را از context بگیرید
    if requestID, ok := ctx.Value("request_id").(string); ok {
        h.logger.Debug("شروع query", "request_id", requestID)
    }
}

func (h *auditHook) AfterQuery(ctx context.Context, query string, args []any, d time.Duration, err error) {
    if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(query)), "DELETE") {
        // لاگ ویژه برای DELETE ها
        h.logger.Warn("عملیات DELETE اجرا شد", "duration", d, "error", err)
    }
}
```

---

### ۹. Migration

#### ساختار فایل‌های Migration

```
migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_add_user_roles.up.sql
├── 000002_add_user_roles.down.sql
└── 000003_create_orders.up.sql
    000003_create_orders.down.sql
```

#### نمونه فایل Migration

```sql
-- migrations/000002_add_user_roles.up.sql
ALTER TABLE users ADD COLUMN role VARCHAR(50) NOT NULL DEFAULT 'user';
CREATE INDEX idx_users_role ON users(role);
```

```sql
-- migrations/000002_add_user_roles.down.sql
DROP INDEX IF EXISTS idx_users_role;
ALTER TABLE users DROP COLUMN IF EXISTS role;
```

#### اجرا از طریق CLI

```bash
# تنظیم متغیر محیطی
export DATABASE_URL="postgres://user:pass@localhost:5432/mydb?sslmode=disable"
export MIGRATIONS_PATH="./migrations"  # اختیاری، پیش‌فرض ./migrations

# اعمال تمام migration های معلق
go run ./cmd/migrate up

# برگشت به migration قبلی
go run ./cmd/migrate down

# برگشت ۳ migration
go run ./cmd/migrate down 3

# مشاهده نسخه فعلی
go run ./cmd/migrate version

# رفع dirty state (در صورت crash در وسط migration)
go run ./cmd/migrate force 2

# حذف کامل (فقط در development!)
go run ./cmd/migrate drop
```

#### اجرای برنامه‌نویسی (Programmatic)

```go
// در صورتی که می‌خواهید migration را هنگام startup اجرا کنید
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func runMigrations(databaseURL string) error {
    m, err := migrate.New("file://./migrations", databaseURL)
    if err != nil {
        return fmt.Errorf("ساخت migrator: %w", err)
    }
    defer m.Close()

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("اجرای migration: %w", err)
    }

    log.Println("Migration ها با موفقیت اعمال شدند")
    return nil
}
```

---

### ۱۰. Driver سفارشی

```go
// اضافه کردن پشتیبانی از CockroachDB
type CockroachDriver struct{}

func (CockroachDriver) Name() string { return "crdb" }

func (CockroachDriver) DSN(o db.DriverOptions) (string, error) {
    port := o.Port
    if port == 0 {
        port = 26257 // پورت پیش‌فرض CockroachDB
    }
    return fmt.Sprintf(
        "postgresql://%s:%s@%s:%d/%s?sslmode=%s",
        o.User, o.Password, o.Host, port, o.Database,
        orDefault(o.SSLMode, "disable"),
    ), nil
}

func (CockroachDriver) ErrorMapper() db.ErrorMapper {
    return db.ChainMapper(crdbSpecificMapper{}, db.DefaultErrorMapper())
}

func (CockroachDriver) Register() {
    // CockroachDB از driver postgres استفاده می‌کند
    // نیازی به ثبت جداگانه نیست
}

// ثبت در init
func init() {
    db.ReplaceDriver(CockroachDriver{})
}
```

---

## 🧪 تست‌نویسی

### اجرای تست‌ها

```bash
# تمام تست‌ها (SQLite in-memory، بدون Docker)
go test ./... -race -v

# فقط db layer
go test ./db/... -race -v

# فقط repo layer
go test ./repo/... -race -v

# با coverage
go test ./... -race -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Unit Test — Mock با interface

چون `UserRepository` یک interface است، می‌توانید با `mockgen` mock بسازید:

```bash
go install github.com/golang/mock/mockgen@latest
mockgen -source=repo/user_repo.go -destination=mocks/user_repo_mock.go -package=mocks
```

```go
// مثال unit test با mock
func TestUserService_GetUser_NotFound(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := mocks.NewMockUserRepository(ctrl)
    mockRepo.EXPECT().
        GetByID(gomock.Any(), int64(999)).
        Return(nil, db.ErrNotFound)

    svc := NewUserService(mockRepo)
    _, err := svc.GetUser(context.Background(), 999)

    if !db.IsNotFound(err) {
        t.Fatalf("expected ErrNotFound, got %v", err)
    }
}
```

### Integration Test — با SQLite

```go
func newTestDB(t *testing.T) *db.DB {
    t.Helper()
    d, err := db.Open(db.Config{
        DSN:        ":memory:", // SQLite in-memory
        DriverName: "sqlite3",
    })
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    t.Cleanup(func() { _ = d.Close() })

    // schema
    _, err = d.Exec(context.Background(), `
        CREATE TABLE users (
            id         INTEGER PRIMARY KEY AUTOINCREMENT,
            name       TEXT NOT NULL,
            email      TEXT NOT NULL UNIQUE,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        )`)
    if err != nil {
        t.Fatalf("schema: %v", err)
    }
    return d
}

func TestUserRepo_Insert_And_GetByID(t *testing.T) {
    database := newTestDB(t)
    r := repo.NewUserRepo(database)
    ctx := context.Background()

    // Insert
    created, err := r.Insert(ctx, models.CreateUserParams{
        Name:  "تست",
        Email: "test@example.com",
    })
    if err != nil {
        t.Fatalf("insert: %v", err)
    }

    // GetByID
    found, err := r.GetByID(ctx, created.ID)
    if err != nil {
        t.Fatalf("get: %v", err)
    }
    if found.Email != "test@example.com" {
        t.Errorf("email اشتباه: %q", found.Email)
    }
}
```

---

## 🏗️ تصمیمات معماری

| تصمیم | دلیل |
|---|---|
| `database/sql` به جای `pgx` مستقیم | driver-agnostic؛ pgx از طریق `pgx/stdlib` قابل استفاده است |
| `Querier` interface | `*DB` و `*Tx` بدون تغییر کد در repo قابل جایگزینی هستند |
| `ErrorMapper` interface | کدهای error مختص driver بدون وابستگی import |
| `Hook` interface | بدون overhead؛ hooks فقط اگر ثبت شده باشند اجرا می‌شوند |
| SQL ثابت در `const` | قابل مشاهده، قابل grep، قابل code review، بدون runtime parse |
| Partial update با `*string` | type-safe؛ از zero-value اشتباه جلوگیری می‌کند |
| `BatchExec` generic | برای هر نوع ردیف بدون reflection کار می‌کند |
| جداسازی Migration CLI | migration هرگز با کد runtime مخلوط نمی‌شود |
| panic recovery در hooks | یک hook معیوب کل اپلیکیشن را crash نمی‌کند |

---

## ⚡ نکات پرفورمنس

- **بدون reflection** در مسیر اصلی اجرا — overhead صفر
- **Hook dispatch** از طریق slice از پیش تخصیص‌یافته، بدون boxing اضافه
- **Prepared statement** ها توسط connection pool driver cache می‌شوند
- **BatchExec** از یک prepared statement برای تمام ردیف‌ها استفاده می‌کند
- **ErrorMapper** بلافاصله روی `nil` short-circuit می‌کند
- **Connection Pool** با تنظیمات دقیق از ساخت/بستن اتصال اضافه جلوگیری می‌کند
- **DefaultTimeout** فقط وقتی context بدون deadline باشد اعمال می‌شود

---

## 📊 مقایسه با ابزارهای مشابه

| ویژگی | sqltoolkit | GORM | sqlx | sqlc |
|---|:---:|:---:|:---:|:---:|
| SQL Explicit | ✅ | ❌ | ✅ | ✅ |
| بدون ORM | ✅ | ❌ | ✅ | ✅ |
| Type-safe errors | ✅ | ❌ | ❌ | ❌ |
| Transaction Helper | ✅ | ✅ | ❌ | ❌ |
| Hook System | ✅ | ✅ | ❌ | ❌ |
| Pluggable Driver | ✅ | ✅ | ❌ | ❌ |
| Migration | ✅ | ✅ | ❌ | ❌ |
| Batch Operations | ✅ | ✅ | ❌ | ❌ |
| Retry Built-in | ✅ | ❌ | ❌ | ❌ |
| بدون Code Generation | ✅ | ✅ | ✅ | ❌ |
| Querier Interface | ✅ | ❌ | ❌ | ❌ |

---

## 📜 لایسنس

MIT License — برای استفاده آزاد در پروژه‌های شخصی و تجاری.

---

<div align="center">
ساخته شده با ❤️ برای جامعه Go
</div>

</div>