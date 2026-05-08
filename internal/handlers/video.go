package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gosocket/internal/hub"
	"gosocket/internal/models"
	"gosocket/internal/repository"
	"gosocket/internal/services"

	"github.com/gin-gonic/gin"
)

type VideoHandler struct {
	repo        *repository.VideoRepo
	transcoder  *services.Transcoder
	hub         *hub.Hub
	storagePath string
}

func NewVideoHandler(repo *repository.VideoRepo, transcoder *services.Transcoder, h *hub.Hub, storagePath string) *VideoHandler {
	return &VideoHandler{repo: repo, transcoder: transcoder, hub: h, storagePath: storagePath}
}

// POST /api/videos — create video record, returns ID before upload begins.
// Body: {"title": "My Video", "filename": "movie.mp4"}
func (vh *VideoHandler) Create(c *gin.Context) {
	var req struct {
		Title    string `json:"title"    binding:"required"`
		Filename string `json:"filename" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	video, err := vh.repo.Create(c.Request.Context(), req.Title, req.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, video)
}

// PUT /api/videos/:id/upload — stream raw video bytes; broadcasts upload_progress via WS.
// Content-Type: video/mp4 (or any video format)
func (vh *VideoHandler) Upload(c *gin.Context) {
	id := c.Param("id")
	video, err := vh.repo.GetByID(c.Request.Context(), id)
	if err != nil || video == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}

	videoDir := filepath.Join(vh.storagePath, id)
	if err := os.MkdirAll(videoDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ext := filepath.Ext(video.Filename)
	if ext == "" {
		ext = ".mp4"
	}
	dstPath := filepath.Join(videoDir, "original"+ext)
	dst, err := os.Create(dstPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer dst.Close()

	if err := vh.repo.SetStatus(c.Request.Context(), id, models.StatusUploading); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pr := &progressReader{
		r:       c.Request.Body,
		total:   c.Request.ContentLength,
		videoID: id,
		hub:     vh.hub,
	}
	if _, err := io.Copy(dst, pr); err != nil {
		_ = vh.repo.SetStatus(c.Request.Context(), id, models.StatusFailed)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
		return
	}

	vh.hub.Broadcast(id, "upload_progress", map[string]float64{"percent": 100}, nil)
	vh.transcoder.Start(id, dstPath, videoDir)
	c.JSON(http.StatusOK, gin.H{"message": "upload complete, transcoding started"})
}

// GET /api/videos/:id — return video metadata.
func (vh *VideoHandler) Get(c *gin.Context) {
	video, err := vh.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil || video == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	c.JSON(http.StatusOK, video)
}

// GET /hls/:id/*filepath — serve HLS master playlist, resolution playlists, and .ts segments.
// Supports HTTP range requests for video seeking.
func (vh *VideoHandler) ServeHLS(c *gin.Context) {
	id := c.Param("id")
	fp := c.Param("filepath")

	if strings.Contains(fp, "..") {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	fullPath := filepath.Join(vh.storagePath, id, filepath.Clean(fp))

	switch {
	case strings.HasSuffix(fp, ".m3u8"):
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
		c.Header("Cache-Control", "no-cache")
	case strings.HasSuffix(fp, ".ts"):
		c.Header("Content-Type", "video/mp2t")
		c.Header("Cache-Control", "max-age=86400")
	}

	http.ServeFile(c.Writer, c.Request, fullPath)
}

// progressReader wraps an io.Reader and broadcasts upload progress via WebSocket.
type progressReader struct {
	r       io.Reader
	total   int64
	read    int64
	videoID string
	hub     *hub.Hub
	lastAt  time.Time
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	pr.read += int64(n)
	if pr.total > 0 && time.Since(pr.lastAt) > 150*time.Millisecond {
		pct := float64(pr.read) / float64(pr.total) * 100
		pr.hub.Broadcast(pr.videoID, "upload_progress", map[string]interface{}{
			"percent": pct,
			"bytes":   pr.read,
			"total":   pr.total,
		}, nil)
		pr.lastAt = time.Now()
	}
	return
}
