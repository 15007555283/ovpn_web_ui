package app

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"ovpn-web-ui/internal/version"
)

type VersionStatus struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Available  bool   `json:"update_available"`
	ReleaseURL string `json:"release_url"`
}

func latestVersionStatus(client *http.Client, current string) (VersionStatus, error) {
	r, err := getRelease(client, "")
	if err != nil {
		return VersionStatus{}, err
	}
	return VersionStatus{Current: current, Latest: r.Tag, Available: r.Tag != current && !olderRelease(r.Tag, current), ReleaseURL: "https://github.com/15007555283/ovpn_web_ui/releases/tag/" + r.Tag}, nil
}

func (a *App) checkVersion(c *gin.Context) {
	// 复用 API 串行锁保护缓存，避免页面刷新重复消耗 GitHub 请求额度。
	if a.versionStatus != nil && time.Since(a.versionChecked) < time.Minute {
		success(c, a.versionStatus)
		return
	}
	client := releaseClient()
	client.Timeout = 5 * time.Second
	status, err := latestVersionStatus(client, version.Value)
	if err != nil {
		failure(c, http.StatusBadGateway, "VERSION_CHECK_FAILED", err.Error())
		return
	}
	a.versionStatus, a.versionChecked = &status, time.Now()
	success(c, status)
}
