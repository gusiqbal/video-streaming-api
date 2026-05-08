package services

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gosocket/internal/hub"
	"gosocket/internal/models"
	"gosocket/internal/repository"
)

type resolution struct {
	name         string
	width        int
	height       int
	videoBitrate string
	maxRate      string
	bufSize      string
	audioBitrate string
	bandwidth    int
}

var resolutions = []resolution{
	{"360p", 640, 360, "800k", "856k", "1200k", "96k", 800_000},
	{"480p", 854, 480, "1400k", "1498k", "2100k", "128k", 1_400_000},
	{"720p", 1280, 720, "2800k", "2996k", "4200k", "128k", 2_800_000},
	{"1080p", 1920, 1080, "5000k", "5350k", "7500k", "192k", 5_000_000},
}

type Transcoder struct {
	ffmpegPath  string
	ffprobePath string
	repo        *repository.VideoRepo
	hub         *hub.Hub
}

func NewTranscoder(ffmpegPath, ffprobePath string, repo *repository.VideoRepo, h *hub.Hub) *Transcoder {
	return &Transcoder{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		repo:        repo,
		hub:         h,
	}
}

// Start launches transcoding in a background goroutine.
func (t *Transcoder) Start(videoID, inputPath, outputDir string) {
	go func() {
		if err := t.run(videoID, inputPath, outputDir); err != nil {
			log.Printf("transcode %s: %v", videoID, err)
			_ = t.repo.SetStatus(context.Background(), videoID, models.StatusFailed)
			t.hub.Broadcast(videoID, "transcoding_failed", map[string]string{"error": err.Error()}, nil)
		}
	}()
}

func (t *Transcoder) run(videoID, inputPath, outputDir string) error {
	duration, err := t.probeDuration(inputPath)
	if err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}

	if err := t.repo.SetStatus(context.Background(), videoID, models.StatusTranscoding); err != nil {
		return err
	}
	t.hub.Broadcast(videoID, "transcoding_started", map[string]float64{"duration": duration}, nil)

	var done []string
	for _, res := range resolutions {
		resDir := filepath.Join(outputDir, res.name)
		if err := os.MkdirAll(resDir, 0755); err != nil {
			return err
		}
		if err := t.transcodeRes(videoID, inputPath, resDir, res, duration); err != nil {
			log.Printf("skip %s for %s: %v", res.name, videoID, err)
			continue
		}
		done = append(done, res.name)
		t.hub.Broadcast(videoID, "transcoding_complete", map[string]string{"resolution": res.name}, nil)
	}

	if len(done) == 0 {
		return fmt.Errorf("all resolutions failed")
	}

	if err := writeMasterPlaylist(outputDir, done); err != nil {
		return err
	}

	if err := t.repo.SetReady(context.Background(), videoID, done, duration); err != nil {
		return err
	}
	t.hub.Broadcast(videoID, "video_ready", map[string]interface{}{
		"resolutions": done,
		"duration":    duration,
	}, nil)
	return nil
}

func (t *Transcoder) transcodeRes(videoID, inputPath, resDir string, res resolution, duration float64) error {
	playlist := filepath.Join(resDir, "index.m3u8")
	segments := filepath.Join(resDir, "%03d.ts")

	args := []string{
		"-loglevel", "error",
		"-nostats",
		"-progress", "pipe:1",
		"-i", inputPath,
		"-vf", fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=decrease", res.width, res.height),
		"-c:v", "libx264",
		"-profile:v", "main",
		"-crf", "23",
		"-g", "48",
		"-keyint_min", "48",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-ar", "48000",
		"-b:a", res.audioBitrate,
		"-b:v", res.videoBitrate,
		"-maxrate", res.maxRate,
		"-bufsize", res.bufSize,
		"-hls_time", "4",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", segments,
		playlist,
	}

	cmd := exec.Command(t.ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	// Parse FFmpeg progress output (key=value lines on stdout via -progress pipe:1).
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "out_time_ms=") {
			continue
		}
		ms, _ := strconv.ParseInt(line[12:], 10, 64)
		if ms <= 0 || duration <= 0 {
			continue
		}
		pct := float64(ms) / 1000 / duration * 100
		if pct > 100 {
			pct = 100
		}
		t.hub.Broadcast(videoID, "transcoding_update", map[string]interface{}{
			"resolution": res.name,
			"percent":    pct,
		}, nil)
	}

	return cmd.Wait()
}

func (t *Transcoder) probeDuration(path string) (float64, error) {
	out, err := exec.Command(t.ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func writeMasterPlaylist(outputDir string, done []string) error {
	resMap := make(map[string]resolution)
	for _, r := range resolutions {
		resMap[r.name] = r
	}

	var sb strings.Builder
	sb.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, name := range done {
		r := resMap[name]
		sb.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n%s/index.m3u8\n",
			r.bandwidth, r.width, r.height, name))
	}
	return os.WriteFile(filepath.Join(outputDir, "master.m3u8"), []byte(sb.String()), 0644)
}
