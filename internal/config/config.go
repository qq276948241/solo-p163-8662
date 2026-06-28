package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBCharset  string
	DBParseTime string
	DBLoc      string

	ServerHost string
	ServerPort string

	JWTSecret     string
	JWTExpireHours int

	AppointmentTimeoutMinutes int
	CronIntervalSeconds       int
}

var AppConfig *Config

func LoadConfig() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using default values")
	}

	AppConfig = &Config{
		DBHost:      getEnv("DB_HOST", "127.0.0.1"),
		DBPort:      getEnv("DB_PORT", "3306"),
		DBUser:      getEnv("DB_USER", "root"),
		DBPassword:  getEnv("DB_PASSWORD", "123456"),
		DBName:      getEnv("DB_NAME", "clinic_appointment"),
		DBCharset:   getEnv("DB_CHARSET", "utf8mb4"),
		DBParseTime: getEnv("DB_PARSE_TIME", "True"),
		DBLoc:       getEnv("DB_LOC", "Local"),

		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "8080"),

		JWTSecret:     getEnv("JWT_SECRET", "clinic_appointment_secret_key_2024"),
		JWTExpireHours: getEnvAsInt("JWT_EXPIRE_HOURS", 24),

		AppointmentTimeoutMinutes: getEnvAsInt("APPOINTMENT_TIMEOUT_MINUTES", 15),
		CronIntervalSeconds:       getEnvAsInt("CRON_INTERVAL_SECONDS", 60),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func GetDSN() string {
	return AppConfig.DBUser + ":" + AppConfig.DBPassword + "@tcp(" + AppConfig.DBHost + ":" + AppConfig.DBPort + ")/" + AppConfig.DBName + "?charset=" + AppConfig.DBCharset + "&parseTime=" + AppConfig.DBParseTime + "&loc=" + AppConfig.DBLoc
}

func GetDSNWithoutDB() string {
	return AppConfig.DBUser + ":" + AppConfig.DBPassword + "@tcp(" + AppConfig.DBHost + ":" + AppConfig.DBPort + ")/?charset=" + AppConfig.DBCharset + "&parseTime=" + AppConfig.DBParseTime + "&loc=" + AppConfig.DBLoc
}
