package router

import (
	"github.com/kataras/iris/v12"
	"iptv-spider-sh/router/api"
)

func InitRouters(app *iris.Application) {
	registerMacros(app)
	// 静态文件路由
	app.HandleDir("/logo", "./logo")
	// 各个路由分组
	app.Get("/epg.xml", func(ctx iris.Context) { ctx.Redirect("/api/epg.xml", iris.StatusMovedPermanently) })
	apiRouterGroup := app.Party("/api")
	{
		api.InitApiRouters(apiRouterGroup)
	}

	// 注册根路径直达路由（兼容所有不带 /api/ 前缀的客户端请求）
	rootRouterGroup := app.Party("/")
	{
		api.InitApiRouters(rootRouterGroup)
	}
}
