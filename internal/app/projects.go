package app

import (
	"github.com/gin-gonic/gin"
	"strings"
)

func (a *App) hasProject(id string) bool {
	if id == "" {
		return true
	}
	for _, p := range a.state.Projects {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (a *App) saveProject(c *gin.Context) {
	var in struct {
		Name   string `json:"name"`
		Remark string `json:"remark"`
	}
	if !body(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.Name == "未分组" || len([]rune(in.Name)) > 80 || len(in.Remark) > 1000 {
		failure(c, 400, "INVALID_INPUT", "项目名称需为 1～80 字，不能为未分组；备注最多 1000 字节")
		return
	}
	id := c.Param("id")
	index := -1
	for i, p := range a.state.Projects {
		if p.ID == id {
			index = i
		}
		if p.ID != id && strings.EqualFold(p.Name, in.Name) {
			failure(c, 409, "PROJECT_EXISTS", "项目名称已存在")
			return
		}
	}
	if id != "" && index < 0 {
		failure(c, 404, "NOT_FOUND", "项目不存在")
		return
	}
	if id == "" && len(a.state.Projects) >= 200 {
		failure(c, 400, "INVALID_INPUT", "最多创建 200 个项目")
		return
	}
	old := a.state.Projects
	a.state.Projects = append([]RouteProject(nil), old...)
	project := RouteProject{ID: id, Name: in.Name, Remark: in.Remark}
	if index < 0 {
		project.ID = token()[:16]
		a.state.Projects = append(a.state.Projects, project)
	} else {
		a.state.Projects[index] = project
	}
	err := a.save()
	if err != nil {
		a.state.Projects = old
	}
	a.finish(c, "project-save", project.ID, project, err)
}

func (a *App) deleteProject(c *gin.Context) {
	for i, p := range a.state.Projects {
		if p.ID != c.Param("id") {
			continue
		}
		// 先移出规则再删项目，避免误删或悄悄改变规则归属。
		for _, r := range a.state.Routes {
			if r.ProjectID == p.ID {
				failure(c, 409, "PROJECT_NOT_EMPTY", "请先将项目内规则移至其他项目或未分组")
				return
			}
		}
		old := a.state.Projects
		a.state.Projects = append(append([]RouteProject{}, old[:i]...), old[i+1:]...)
		err := a.save()
		if err != nil {
			a.state.Projects = old
		}
		a.finish(c, "project-delete", p.ID, nil, err)
		return
	}
	failure(c, 404, "NOT_FOUND", "项目不存在")
}
