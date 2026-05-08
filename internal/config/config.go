package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	StoragePath string
	FFmpegPath  string
	FFprobePath string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/videodb?sslmode=disable"),
		StoragePath: getEnv("STORAGE_PATH", "./storage/videos"),
		FFmpegPath:  getEnv("FFMPEG_PATH", "ffmpeg"),
		FFprobePath: getEnv("FFPROBE_PATH", "ffprobe"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
