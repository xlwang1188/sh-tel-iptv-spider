package api

import (
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
	"iptv-spider-sh/global"
	"iptv-spider-sh/modules/auth"
	"iptv-spider-sh/utils"
	"strconv"
	"time"
)

// generateXmlTv 生成xmlTv文件 节目去重
func generateXmlTv(ctx iris.Context) {
	ref := ctx.FormValue("ref")
	daysAgo := ctx.FormValueDefault("daysAgo", "1")

	d, err := strconv.Atoi(daysAgo)
	if err != nil {
		ctx.WriteString(err.Error())
		return
	}

	// 缓存
	reqMD5Key := utils.CalcMD5KeyForRequest("generateXmlTv", daysAgo)
	if ref != "true" && global.CACHE.IsExist(reqMD5Key) {
		// 存在缓存，直接返回
		ctx.ContentType(context.ContentXMLHeaderValue)
		ctx.Write(global.CACHE.Get(reqMD5Key).([]byte))
		return
	}
	// 并发时合并请求
	resp, err, _ := global.ConcurrencyControl.Do(reqMD5Key, func() (interface{}, error) {
		epgBytes, err := auth.GenerateXmlTv(d)
		timeOut := time.Duration(global.CONFIG.Cache.DefTimeOut)
		global.CACHE.Put(reqMD5Key, epgBytes, time.Minute*timeOut)
		return epgBytes, err
	})
	if err != nil {
		ctx.WriteString(err.Error())
		return
	}
	ctx.ContentType(context.ContentXMLHeaderValue)
	ctx.Write(resp.([]byte))
}
