package auth

import (
	"encoding/hex"
	"fmt"
	"iptv-spider-sh/global"
	"iptv-spider-sh/utils"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/robertkrimen/otto"
	"go.uber.org/zap"
)

func (c *Client) epgIndex(doc *goquery.Document) *goquery.Document {
	uri, method, formMap := utils.GetFromParamByHtml(doc, "form#epgform")
	// 保存 Token
	c.UserToken = formMap["UserToken"]
	resp := c.httpClient.Request(uri, method, formMap)
	return utils.CreateHtmlDocByBytes(uri, resp.GetRespBytes())
}

func (c *Client) epgLoadBalance(doc *goquery.Document) *goquery.Document {
	var uri string
	scs := utils.GetScriptsFormHtml(doc)
	for _, sc := range scs {
		if !strings.Contains(sc, "top.document.location") {
			continue
		}
		sArr := strings.Split(sc, "\n")
		for _, s := range sArr {
			if !strings.Contains(s, "top.document.location") {
				continue
			}
			index := strings.Index(s, "'")
			last := strings.LastIndex(s, "'")
			if index < 0 || last < 0 {
				global.LOG.Error("No top.document.location")
				return nil
			}
			uri = s[index+1 : last]
		}
	}
	u, err := url.Parse(uri)
	if err != nil {
		global.LOG.Error(err.Error())
		return nil
	}
	c.EPGLoginHost = u.Host
	resp := c.httpClient.Request(uri, "GET", nil)
	return utils.CreateHtmlDocByBytes(uri, resp.GetRespBytes())
}

func (c *Client) epgPortalAuth(doc *goquery.Document) (*goquery.Document, error) {
	// 重试次数，最大设为3次
	maxRetries := 3
	var lastErr error

	for retries := 0; retries < maxRetries; retries++ {
		uri, method, formMap := utils.GetFromParamByHtml(doc, "form")

		r := utils.RSA{}
		r.LoadPriKey(utils.GetRSAPriKey())

		token := formMap["UserToken"]
		plainData := utils.InsertStrInUserToken(token)
		if plainData == "" {
			global.LOG.Error("InsertStrInUserToken: plainData is empty, Token: " + token)
			return nil, fmt.Errorf("token is empty")
		}
		stbInfo := hex.EncodeToString(r.PriEncrypt([]byte(plainData)))
		formMap["stbtype"] = c.stbType
		formMap["stbinfo"] = strings.ToUpper(stbInfo)

		resp := c.httpClient.Request(uri, method, formMap)
		respDoc := utils.CreateHtmlDocByBytes(uri, resp.GetRespBytes())

		infoMap := c.parseEpgAuthInfo(respDoc)
		c.JSESSIONID = infoMap["SessionID"]
		c.EPGHostUrl = fmt.Sprintf("http://%s/iptvepg/%s", infoMap["IpPort"], infoMap["framecode"])

		// 校验 EPGHostUrl
		if !isValidEPGHostUrl(c.EPGHostUrl) {
			global.LOG.Error("生成的EPGHostUrl格式无效: " + c.EPGHostUrl)
			lastErr = fmt.Errorf("EPG地址格式无效: %s", c.EPGHostUrl)

			// 如果不是最后一次尝试，打印重试信息并继续
			if retries < maxRetries-1 {
				global.LOG.Info("EPGHostUrl 格式无效，正在重试...", zap.Int("RetryAttempt", retries+1))
				continue
			}
			return nil, lastErr
		}

		global.LOG.Info("EPG门户认证成功", zap.String("EPGHostUrl", c.EPGHostUrl))
		return respDoc, nil
	}

	return nil, lastErr // 如果最终没有成功，则返回最后的错误
}

// isValidEPGHostUrl 检查 EPGHostUrl 是否符合标准格式
func isValidEPGHostUrl(urlStr string) bool {
	// 尝试解析 URL 并检查是否符合预期格式
	u, err := url.Parse(urlStr)
	if err != nil {
		global.LOG.Error("EPGHostUrl 解析失败: ", zap.Any("url", urlStr))
		return false
	}

	// 检查 URL 的 Host 部分是否是有效的 IP 或域名
	if u.Host == "" || !strings.Contains(u.Host, ":") {
		return false
	}

	// 检查是否符合期望的 URL 格式，例如: http://218.83.188.230:8084/iptvepg/frame1002
	if !strings.HasPrefix(urlStr, "http://") {
		return false
	}

	return true
}

func (c *Client) parseEpgAuthInfo(doc *goquery.Document) map[string]string {
	cache := map[string]string{}
	c.jsVM.Reset()
	c.jsVM.RunScriptForHtml(doc)
	c.jsVM.Set("jsSetConfig", func(call otto.FunctionCall) otto.Value {
		k := call.Argument(0).String()
		v := call.Argument(1).String()
		cache[k] = v
		return otto.Value{}
	})
	scs := utils.GetScriptsFormHtml(doc)
	for _, sc := range scs {
		sArr := strings.Split(sc, "\n")
		for _, s := range sArr {
			if !strings.Contains(s, "jsSetConfig") {
				continue
			}
			c.jsVM.RunScript(s)
		}
	}
	// 打印解析到的配置信息
	global.LOG.Info(fmt.Sprintf("解析到的IPTV配置信息: %+v", cache))

	return cache
}

func (c *Client) epgGetPortal() {
	// http://218.83.165.40:8084/iptvepg/frame1413/portal.jsp
	p := "portal.jsp"
	uri := fmt.Sprintf("%s/%s", c.EPGHostUrl, p)
	c.httpClient.Request(uri, "GET", nil)
}
