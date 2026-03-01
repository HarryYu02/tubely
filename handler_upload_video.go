package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func validateVideoType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "video/mp4":
		return true
	default:
		return false
	}
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
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

	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

	fmt.Println("uploading video for video", videoID, "by user", userID)

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video cannot be retrieved", err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not the video owner", err)
		return
	}

	err = r.ParseMultipartForm(uploadLimit)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Form data cannot be parsed", err)
		return
	}
	fileData, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video data cannot be read", err)
		return
	}
	defer fileData.Close()
	mediaType := fileHeader.Header.Get("Content-Type")

	isValidVideo := validateVideoType(mediaType)
	if !isValidVideo {
		respondWithError(w, http.StatusBadRequest, "Media type not allowed", err)
		return
	}

	videoExt := "mp4"
	key := make([]byte, 32)
	rand.Read(key)
	keyStr := base64.RawURLEncoding.EncodeToString(key)
	keyStrWithExt := keyStr + "." + videoExt
	videoFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be saved", err)
		return
	}
	defer os.Remove(videoFile.Name())
	defer videoFile.Close()
	_, err = io.Copy(videoFile, fileData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be saved", err)
		return
	}
	videoFile.Seek(0, io.SeekStart)

	processedVideoPath, err := processVideoForFastStart(videoFile.Name())
	processedVideoFile, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be processed for fast start", err)
		return
	}
	defer os.Remove(processedVideoFile.Name())
	defer processedVideoFile.Close()

	aspectRatio, err := getVideoAspectRatio(videoFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video's aspect ratio cannot be determined", err)
		return
	}
	prefix := "other"
	switch aspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "portrait"
	}
	fullKeyStr := fmt.Sprintf("%s/%s", prefix, keyStrWithExt)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fullKeyStr,
		Body: processedVideoFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be saved to aws", err)
		return
	}
	videoURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, fullKeyStr)
	videoData.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be updated", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoData)
}
