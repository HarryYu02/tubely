package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

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
	}
	fileData, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Thumbnail data cannot be read", err)
	}
	mediaType := fileHeader.Header.Get("Content-Type")
	imageData, err := io.ReadAll(fileData)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Thumbnail cannot be read", err)
	}
	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video cannot be retrieved", err)
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not the video owner", err)
	}

	thumbnailData := thumbnail{
		data: imageData,
		mediaType: mediaType,
	}
	videoThumbnails[videoID] = thumbnailData
	thumbnailURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoID.String())
	videoData.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be updated", err)
	}

	videoBytes, err := json.Marshal(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Video cannot be marsalled", err)
	}

	respondWithJSON(w, http.StatusOK, videoBytes)
}
