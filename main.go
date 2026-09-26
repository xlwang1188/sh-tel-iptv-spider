package main

import (
	"github.com/golang-module/carbon"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/recover"
	"github.com/robfig/cron/v3"
	"iptv-spider-sh/global"
	"iptv-spider-sh/initialize"
	"iptv-spider-sh/modules/auth"
	"iptv-spider-sh/router"
)

func main() {
	initialize.Viper()
	global.LOG = initialize.Zap()
	global.CACHE = initialize.Cache()
	global.DB = initialize.Gorm()
	ossType := global.CONFIG.System.OSSType
	if ossType == "cos" {
		global.COS = initialize.COS()
	} else if ossType == "minio" {
		global.MinioClient = initialize.Minio()
	}

	initialize.InitCron()

	if global.DB != nil {
		initialize.MysqlTables(global.DB) // 初始化表
		// 程序结束前关闭数据库链接
		db, _ := global.DB.DB()
		defer db.Close()
	}

	app := iris.New()

	app.Use(recover.New())

	router.InitRouters(app)

	stb := global.CONFIG.Stb
	var opts []auth.ClientOption
	if stb.Type != "" {
		opts = append(opts, auth.WithSTBType(stb.Type))
	}
	if stb.AuthHost != "" {
		opts = append(opts, auth.With4kLogAuthAddr(stb.AuthHost))
	}
	client, err := auth.NewGlobalClient(stb.UID, stb.SN, stb.MAC, stb.IP, opts...)
	if err != nil {
		global.LOG.Error("New AuthClient: " + err.Error())
		return
	}
	addSpiderCron(client)
	addDataCleanCron()
	addUploadCosCron()

	// 启动一个协程来运行
	go func() {
		// 启动时获取频道列表
		client.FetchChannelList()
		// 拉取一次节目单，如果近期更新过，则不会实际运行
		client.FetchChannelProg()
	}()

	app.ConfigureHost(configHost)
	_ = app.Run(iris.Addr(global.CONFIG.System.Addr), iris.WithoutServerError(iris.ErrServerClosed))

}

// 爬虫定时器
func addSpiderCron(c *auth.Client) {
	epgConfig := global.CONFIG.Epg
	var listID, progID cron.EntryID
	listID, _ = global.CRON.AddFunc("@daily", func() {
		c.FetchChannelList()
		logNexTime("Task: 获取频道信息列表;\t\tNextTime: ", listID)
	})
	logNexTime("Add Task: 获取频道信息列表;\t\tNextTime: ", listID)

	progID, _ = global.CRON.AddFunc(epgConfig.FetchCron, func() {
		c.FetchChannelProg()
		logNexTime("Task: 获取频道节目单列表;\t\tNextTime: ", progID)
	})
	logNexTime("Add Task: 获取频道节目单列表;\t\tNextTime: ", progID)
}

// 数据清理定时器
func addDataCleanCron() {
	var taskId cron.EntryID
	taskId, _ = global.CRON.AddFunc("@every 2h", func() {
		auth.CleanEPGDetailsData()
		auth.CleanChannelInfoData()
		auth.CleanChannelData()
		logNexTime("Task: 清理过期数据;\t\t\tNextTime: ", taskId)
	})
	logNexTime("Add Task: 清理过期数据;\t\t\tNextTime: ", taskId)
}

func addUploadCosCron() {
	ossConfig := global.CONFIG.OSS
	if !ossConfig.Enable || ossConfig.UploadCron == "" {
		return
	}
	var taskId0, taskId1 cron.EntryID
	taskId0, _ = global.CRON.AddFunc(ossConfig.UploadCron, func() {
		auth.GenerateAndUploadM3u()
		auth.GenerateAndUploadXmlTv()
		logNexTime("Task: 上传文件至COS;\t\tNextTime: ", taskId0)
	})
	logNexTime("Add Task: 上传文件至COS;\t\tNextTime: ", taskId0)

	taskId1, _ = global.CRON.AddFunc("0 25 0,12 * * *", func() {
		auth.GenerateAndUploadXmlTvDays7()
		logNexTime("Task: 上传至COS-7D;\t\t\tNextTime: ", taskId1)
	})
	logNexTime("Add Task: 上传至COS-7D;\t\t\tNextTime: ", taskId1)
}

func logNexTime(msg string, id cron.EntryID) {
	t := global.CRON.Entry(id).Next
	global.LOG.Info(msg + carbon.FromStdTime(t).ToString())
}

func configHost(su *iris.Supervisor) {
	su.RegisterOnShutdown(func() {
		println("Server Closed")
	})
}
