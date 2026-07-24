#!/bin/bash

# قطع اسکریپت در صورت بروز هرگونه خطا
set -e

# رنگ‌بندی محیط متنی ترمینال
Reset="\033[0m"
Cyan="\033[36m"
Green="\033[32m"
Yellow="\033[33m"
Red="\033[31m"
Purple="\033[35m"

clear
echo -e "${Cyan}=========================================================${Reset}"
echo -e "${Cyan}             ✨ SVM DISTRIBUTED PANEL CLUSTER ✨          ${Reset}"
echo -e "${Cyan}=========================================================${Reset}"
echo -e "${Green} 1)${Reset} Install/Update MAIN Server (با دامین، Nginx و SSL خودکار)"
echo -e "${Yellow} 2)${Reset} Install NODE Server (نصب نود فرعی)"
echo -e "${Red} 3)${Reset} Clean Uninstall (پاکسازی کامل سیستم)"
echo -e "${Cyan} 4)${Reset} Exit"
echo -e "${Cyan}=========================================================${Reset}"
read -p "Please select an option [1-4]: " main_choice

case $main_choice in
    1)
        echo -e "${Green}\n[1/8] Pre-Installation Configuration...${Reset}"
        read -p "Enter your Domain/Subdomain (e.g., panel.domain.com): " USER_DOMAIN
        if [ -z "$USER_DOMAIN" ]; then
            echo -e "${Red}❌ Error: Domain name cannot be empty!${Reset}"
            exit 1
        fi

        # تولید یک توکن امن و تصادفی برای اتصال نودها
        GENERATED_TOKEN=$(openssl rand -hex 32)
        echo -e "${Yellow}✔️ Secure Cluster Token generated automatically.${Reset}"

        echo -e "${Green}\n[2/8] Updating system and installing dependencies (Nginx & Certbot)...${Reset}"
        apt-get update -y
        apt-get install -y git golang mariadb-server zip unzip curl openssl nginx certbot python3-certbot-nginx

        echo -e "${Green}[3/8] Configuring MariaDB Database...${Reset}"
        systemctl start mariadb
        systemctl enable mariadb

        # اصلاح دیتابیس مطابق کد Go جدید (استفاده از root برای جلوگیری از قطعی)
        mysql -u root -e "CREATE DATABASE IF NOT EXISTS svm_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
        mysql -u root -e "CREATE TABLE IF NOT EXISTS svm_db.settings (key_name VARCHAR(50) PRIMARY KEY, key_value TEXT);"

        echo -e "${Green}[4/8] Fetching latest source code from GitHub...${Reset}"
        if [ -d "/root/svm-panel" ]; then
            cd /root/svm-panel
            git reset --hard
            git pull
        else
            cd /root
            git clone https://github.com/asd1asd00000/svm-panel.git
            cd /root/svm-panel
        fi

        echo -e "${Green}[5/8] Generating secure Web UI Admin credentials...${Reset}"
        ADMIN_USER="admin"
        ADMIN_PASS=$(openssl rand -hex 8)
        BASE_URL="https://$USER_DOMAIN"

        mysql -u root svm_db -e "INSERT INTO settings (key_name, key_value) VALUES ('admin_username', '$ADMIN_USER') ON DUPLICATE KEY UPDATE key_value='$ADMIN_USER';"
        mysql -u root svm_db -e "INSERT INTO settings (key_name, key_value) VALUES ('admin_password', '$ADMIN_PASS') ON DUPLICATE KEY UPDATE key_value='$ADMIN_PASS';"
        mysql -u root svm_db -e "INSERT INTO settings (key_name, key_value) VALUES ('panel_url', '$BASE_URL') ON DUPLICATE KEY UPDATE key_value='$BASE_URL';"

        echo -e "${Green}[6/8] Downloading Go modules and compiling core binary...${Reset}"
        cd /root/svm-panel
        
        # حذف کامل فایل‌های ماژول قبلی برای جلوگیری از تداخل و ساخت مجدد آن‌ها
        rm -f go.mod go.sum
        export GO111MODULE=on
        go mod init github.com/asd1asd00000/svm-panel
        
        # دانلود مستقیم پکیج‌های خارجی ضروری
        go get github.com/go-sql-driver/mysql
        go get golang.org/x/crypto/ssh
        
        go mod tidy
        go build -o svm-panel main.go
        cp svm-panel /usr/local/bin/
        chmod +x /usr/local/bin/svm-panel

        echo -e "${Green}[7/8] Configuring Nginx Reverse Proxy & SSL (Certbot)...${Reset}"
        cat <<EOF > /etc/nginx/sites-available/svm-panel
server {
    listen 80;
    server_name $USER_DOMAIN;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
        ln -sf /etc/nginx/sites-available/svm-panel /etc/nginx/sites-enabled/
        rm -f /etc/nginx/sites-enabled/default || true
        systemctl restart nginx

        echo -e "${Purple}⏳ Requesting Let's Encrypt SSL for $USER_DOMAIN...${Reset}"
        certbot --nginx --non-interactive --agree-tos --register-unsafely-without-email -d $USER_DOMAIN || true
        systemctl restart nginx

        echo -e "${Green}[8/8] Creating systemd background service for MAIN server...${Reset}"
        # پورت صراحتاً روی 8080 و توکن به صورت متغیر تزریق می‌شود
        cat <<EOF > /etc/systemd/system/svm-api.service
[Unit]
Description=SVM Distributed Panel API & Web Daemon
After=network.target mariadb.service nginx.service

[Service]
Type=simple
User=root
WorkingDirectory=/root/svm-panel
ExecStart=/usr/local/bin/svm-panel -mode api -port 8080 -token $GENERATED_TOKEN
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

        systemctl daemon-reload
        systemctl enable svm-api.service
        systemctl restart svm-api.service
        hash -r

        echo -e "${Cyan}=========================================================${Reset}"
        echo -e "${Green} ✔️ MAIN Server successfully installed & SSL Configured!${Reset}"
        echo -e "${Cyan}=========================================================${Reset}"
        echo -e " 🌐 Web UI Login    : ${Purple}$BASE_URL/admin/login${Reset}"
        echo -e "---------------------------------------------------------"
        echo -e " 👤 Admin Username  : ${Yellow}$ADMIN_USER${Reset}"
        echo -e " 🔑 Admin Password  : ${Yellow}$ADMIN_PASS${Reset}"
        echo -e "---------------------------------------------------------"
        echo -e "${Yellow} 🚨 COPY THIS DATA FOR NODE INSTALLATION 🚨${Reset}"
        echo -e " 🌐 Main Server URL : ${Green}http://$(curl -s ifconfig.me):8080${Reset} (Or Domain)"
        echo -e " 🔑 Cluster Token   : ${Green}$GENERATED_TOKEN${Reset}"
        echo -e "---------------------------------------------------------"
        echo -e " Run 'svm-panel' anytime in terminal to open the management menu."
        echo -e "${Cyan}=========================================================${Reset}"
        ;;

    2)
        echo -e "${Yellow}\n--- Installing NODE Server Mode ---${Reset}"
        read -p "Enter MAIN Server Base URL (e.g., http://1.2.3.4:8080 or https://panel.com): " main_server_url
        read -p "Enter Cluster Security Token: " cluster_token
        
        if [ -z "$main_server_url" ] || [ -z "$cluster_token" ]; then
            echo -e "${Red}❌ Error: URL and Token cannot be empty!${Reset}"
            exit 1
        fi

        echo -e "${Yellow}⏳ Installing dependencies for Node...${Reset}"
        apt-get update -y
        apt-get install -y git golang curl

        echo -e "${Yellow}⏳ Cloning repository...${Reset}"
        if [ -d "/root/svm-panel" ]; then
            cd /root/svm-panel
            git reset --hard
            git pull
        else
            cd /root
            git clone https://github.com/asd1asd00000/svm-panel.git
            cd /root/svm-panel
        fi

        echo -e "${Yellow}⏳ Downloading Go modules and building Node executable...${Reset}"
        cd /root/svm-panel
        
        # حذف کش و ساختاردهی مجدد پکیج‌ها برای نود
        rm -f go.mod go.sum
        export GO111MODULE=on
        go mod init github.com/asd1asd00000/svm-panel
        
        go get github.com/go-sql-driver/mysql
        go get golang.org/x/crypto/ssh
        
        go mod tidy
        go build -o svm-panel main.go
        cp svm-panel /usr/local/bin/
        chmod +x /usr/local/bin/svm-panel
        hash -r

        echo -e "${Yellow}⏳ Creating systemd background service for NODE server...${Reset}"
        cat <<EOF > /etc/systemd/system/svm-node.service
[Unit]
Description=SVM Distributed Node Daemon
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/svm-panel
ExecStart=/usr/local/bin/svm-panel -mode node -main-url $main_server_url -token $cluster_token
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

        systemctl daemon-reload
        systemctl enable svm-node.service
        systemctl restart svm-node.service

        echo -e "${Green}==========================================${Reset}"
        echo -e "${Green} ✔️ NODE Server successfully connected!    ${Reset}"
        echo -e "${Green}==========================================${Reset}"
        ;;

    3)
        echo -e "${Red}\n⚠️ WARNING: This will completely wipe your database, logs, Nginx configs, and panel files!${Reset}"
        read -p "Are you absolutely sure you want to uninstall? (y/n): " confirm_uninstall
        if [ "$confirm_uninstall" != "y" ] && [ "$confirm_uninstall" != "Y" ]; then
            echo "Uninstall cancelled."
            exit 0
        fi

        echo -e "${Red}⏳ Stopping and purging services...${Reset}"
        systemctl stop svm-api.service || true
        systemctl disable svm-api.service || true
        systemctl stop svm-node.service || true
        systemctl disable svm-node.service || true

        echo -e "${Red}⏳ Removing Nginx configurations...${Reset}"
        rm -f /etc/nginx/sites-enabled/svm-panel || true
        rm -f /etc/nginx/sites-available/svm-panel || true
        systemctl restart nginx || true

        echo -e "${Red}⏳ Removing binaries and systemd scripts...${Reset}"
        rm -f /etc/systemd/system/svm-api.service
        rm -f /etc/systemd/system/svm-node.service
        systemctl daemon-reload

        rm -f /usr/local/bin/svm-panel
        hash -r
        
        echo -e "${Red}⏳ Dropping database and users...${Reset}"
        mysql -u root -e "DROP DATABASE IF EXISTS svm_db;" || true
        
        echo -e "${Red}⏳ Deleting deployment directories...${Reset}"
        rm -rf /root/svm-panel

        echo -e "${Green}==========================================${Reset}"
        echo -e "${Green} ✔️ Clean Uninstall Completed Successfully!${Reset}"
        echo -e "${Green}==========================================${Reset}"
        ;;

    4)
        echo "Exiting installation."
        exit 0
        ;;
    *)
        echo -e "${Red}❌ Invalid Choice!${Reset}"
        exit 1
        ;;
esac
