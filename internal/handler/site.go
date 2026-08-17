package handler

import (
	"os"
	"regexp"
	"strings"

	"github.com/example/epay-go/internal/config"
	"github.com/example/epay-go/pkg/response"
	"github.com/gin-gonic/gin"
)

var gonganCodeRe = regexp.MustCompile(`\d{5,}`)

func GetSite(c *gin.Context) {
	icp := strings.TrimSpace(os.Getenv("BEIAN_ICP"))
	gongan := strings.TrimSpace(os.Getenv("BEIAN_GONGAN"))
	if cfg := config.Get(); cfg != nil {
		if icp == "" {
			icp = strings.TrimSpace(cfg.Site.BeianICP)
		}
		if gongan == "" {
			gongan = strings.TrimSpace(cfg.Site.BeianGongAn)
		}
	}

	gonganURL := ""
	if code := gonganCodeRe.FindString(gongan); code != "" {
		gonganURL = "https://www.beian.gov.cn/portal/registerSystemInfo?recordcode=" + code
	}

	response.Success(c, gin.H{
		"beian_icp":        icp,
		"beian_gongan":     gongan,
		"beian_gongan_url": gonganURL,
	})
}
