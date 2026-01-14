package handlers

import (
	"krampus/server/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetPublicConfig returns public configuration values for the frontend
func GetPublicConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"vote_threshold": config.AppConfig.VoteThreshold,
		"sync_base_url":  config.AppConfig.SyncBaseURL,
	})
}
