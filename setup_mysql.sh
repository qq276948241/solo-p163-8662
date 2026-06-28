#!/bin/bash
set -e

echo "Updating packages..."
apt-get update -qq

echo "Installing MySQL Server..."
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq mysql-server

echo "Starting MySQL..."
service mysql start

echo "Configuring MySQL..."
mysql <<EOF
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY '123456';
CREATE DATABASE IF NOT EXISTS clinic_appointment CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON clinic_appointment.* TO 'root'@'localhost';
FLUSH PRIVILEGES;
EOF

echo "MySQL setup complete!"
echo "MySQL is running on port 3306"
echo "Database: clinic_appointment"
echo "User: root, Password: 123456"
