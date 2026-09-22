package handler

import "github.com/gin-gonic/gin"

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
