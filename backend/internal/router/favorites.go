package router

import (
	"github.com/gin-gonic/gin"
	"github.com/lp/campus-market/internal/handler"
)

// RegisterFavoriteRoutes registers favorite endpoints (all require login).
func RegisterFavoriteRoutes(g *gin.RouterGroup, h *handler.FavoriteHandler, auth, apiLimiter gin.HandlerFunc) {
	favorites := g.Group("/favorites", auth)
	{
		favorites.POST("", apiLimiter, h.Add)
		favorites.GET("/me", apiLimiter, h.ListMine)
		favorites.DELETE("/:productId", apiLimiter, h.Remove)
	}
}
