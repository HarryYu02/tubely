package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"encoding/base64"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func getExt(contentType string) string {
	// Content-Type often includes parameters like "text/html; charset=utf-8"
	// We need to strip everything after the semicolon.
	mediaType := strings.Split(contentType, ";")[0]

	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		return ""
	}

	// Returns the first extension (e.g., "html")
	return exts[0][1:]
}

func validateMimeType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "image/jpeg":
		fallthrough
	case "image/png":
		return true
	default:
		return false
	}
}

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}


	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Form data cannot be parsed", err)
		return
	}
	fileData, fileHeader, err := r.FormFile("thumbnail")
	defer fileData.Close()
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Thumbnail data cannot be read", err)
		return
	}
	mediaType := fileHeader.Header.Get("Content-Type")
	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video cannot be retrieved", err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not the video owner", err)
		return
	}

	isValidThumbnail := validateMimeType(mediaType)
	if !isValidThumbnail {
		respondWithError(w, http.StatusBadRequest, "Media type not allowed", err)
		return
	}

	imageExt := getExt(mediaType)
	key := make([]byte, 32)
	rand.Read(key)
	keyStr := base64.RawURLEncoding.EncodeToString(key)
	thumbnailSubPath := fmt.Sprintf("%s.%s", keyStr, imageExt)
	thumbnailPath := filepath.Join(cfg.assetsRoot, thumbnailSubPath)
	thumbnailFile, err := os.Create(thumbnailPath)
	defer thumbnailFile.Close()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Thumbnail cannot be saved", err)
		return
	}
	_, err = io.Copy(thumbnailFile, fileData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Thumbnail cannot be saved", err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, keyStr, imageExt)
	videoData.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be updated", err)
		return
	}

	videoBytes, err := json.Marshal(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be marsalled", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoBytes)
}
