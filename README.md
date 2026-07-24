# SVM Distributed Panel

پنل مدیریت کاربران VPN مبتنی بر تونل SSH با معماری توزیع‌شده (Main + Node).

## امکانات
- ساخت/مدیریت کاربر (حجم، انقضا، پورت UDPGW، کامنت)
- **پلن نامحدود** (حجم = 0 یعنی بدون محدودیت)
- رهگیری ترافیک ساعتی/روزانه به تفکیک نود
- داشبورد وب (نسخه موبایل و دسکتاپ) + صفحه اشتراک کاربر (`/sub/<token>`)
- مانیتور نودهای متصل و آمار ترافیک هر نود
- بکاپ خودکار و ارسال به تلگرام + بازیابی

## حالت‌های اجرا
```bash
svm-panel                 # منوی مدیریت CLI (پیش‌فرض)
svm-panel -mode api  -port 8080 -token <TOKEN>          # سرور اصلی + وب
svm-panel -mode node -main-url http://IP:8080 -token <TOKEN>   # نود فرعی
```

## نصب
```bash
bash svm-panel.sh    # گزینه ۱ = Main ، گزینه ۲ = Node ، گزینه ۳ = حذف کامل
```

## متغیرهای محیطی (اختیاری)
| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `SVM_DB_DSN` | `root:@tcp(127.0.0.1:3306)/svm_db?parseTime=true` | رشته اتصال MariaDB |
| `SVM_API_PORT` | `8080` | پورت API (برای منوی CLI) |
| `SVM_LOG_PATH` | `/root/svm-panel/system.log` | مسیر فایل لاگ |
| `SVM_SSH_HOST_KEY` | `/root/svm-panel/ssh_host_key` | مسیر کلید میزبان SSH (پایدار) |

## نکات امنیتی
- نشست ادمین با توکن تصادفی سمت سرور مدیریت می‌شود؛ کوکی با `HttpOnly`، `Secure` (روی HTTPS) و `SameSite=Strict`.
- رمز ادمین با **bcrypt** ذخیره می‌شود (رمزهای plaintext قدیمی در اولین ورود موفق خودکار ارتقا می‌یابند).
- توکن کلاستر با مقایسه constant-time بررسی می‌شود و از هدر `X-Auth-Token`/`Authorization` هم پذیرفته می‌شود.
- کلید میزبان SSH فقط یک‌بار ساخته و روی دیسک ذخیره می‌شود.
- بکاپ/بازیابی بدون `sh -c` انجام می‌شود (مصون در برابر command injection).

> توصیه: بعد از نصب حتماً رمز ادمین را از پنل تغییر دهید و برای MariaDB رمز root تنظیم کنید (سپس `SVM_DB_DSN` را ست کنید).

## ساخت از سورس
```bash
go build -o svm-panel main.go
```

## ساختار پروژه
```
main.go             منوی CLI و انتخاب حالت اجرا
models/user.go      تایپ‌های مشترک + ثابت‌ها + هلپرها (IranTime, GB, IsActive ...)
database/mariadb.go لایه دیتابیس، بکاپ، آمار، اعتبارسنجی ادمین
sshvpn/server.go    سرور SSH، احراز هویت، شمارش ترافیک، سینک نود
api/core.go         نشست، امنیت، هلپرها، GeoIP کش‌دار
api/server.go       رجیستر روت‌ها + سرور HTTP با timeout
api/handlers.go     هندلرهای HTTP
api/templates.go    قالب‌های HTML
svm-panel.sh        نصاب (Main/Node/Uninstall)
```
