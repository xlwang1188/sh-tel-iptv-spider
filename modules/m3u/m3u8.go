package m3u

import (
	"fmt"
	"iptv-spider-sh/model"

	"go.uber.org/zap/buffer"
)

type Writer struct {
	buf buffer.Buffer
}

func (m *Writer) WriteHeaderWithInfo(xmlUrl string) {
	// http://10.0.0.10:34400/xmltv/xteve.xml
	if xmlUrl == "" {
		m.WriteHeader()
		return
	}
	m.buf.WriteString(fmt.Sprintf(`#EXTM3U url-tvg="%s" x-tvg-url="%s" tvg-url="%s" catchup="default" catchup-days="7"`, xmlUrl, xmlUrl, xmlUrl))
	m.buf.WriteString("\n")
}

func (m *Writer) WriteHeader() {
	m.buf.WriteString("#EXTM3U catchup=\"default\" catchup-days=\"7\"\n")
}

func (m *Writer) Write(uri string, info model.ChannelInfo, ext model.M3u8Mapping) {
	var groups = ext.CustomGroups
	if groups == "" {
		groups = ext.AutoGroups
	}
	m.buf.WriteString("\n")
	m.buf.WriteString(fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s"`, info.MixNo, info.CommName, ext.Logo))
	m.buf.WriteString(fmt.Sprintf(` group-title="%s"`, groups))
	m.buf.WriteString(fmt.Sprintf(",%s\n%s", info.Name, uri))
}

// 新增方法：带回看地址
func (m *Writer) WriteWithCatchup(uri string, catchup string, info model.ChannelInfo, ext model.M3u8Mapping) {
	var groups = ext.CustomGroups
	if groups == "" {
		groups = ext.AutoGroups
	}

	m.buf.WriteString("\n")
	if catchup != "" {
		// 带回看地址：使用标准 default 回看模式与 catchup-source 模板联动
		m.buf.WriteString(fmt.Sprintf(
			`#EXTINF:-1 tvg-id="%s" tvg-name="%s" catchup="default" catchup-days="7" tvg-rec="7" catchup-source="%s" tvg-logo="%s" group-title="%s",%s`,
			info.MixNo,
			info.CommName,
			catchup,
			ext.Logo,
			groups,
			info.Name,
		))
	} else {
		// 普通EXTINF
		m.buf.WriteString(fmt.Sprintf(
			`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s",%s`,
			info.MixNo,
			info.CommName,
			ext.Logo,
			groups,
			info.Name,
		))
	}
	m.buf.WriteString("\n")
	m.buf.WriteString(uri)
}

func (m *Writer) Bytes() []byte {
	return m.buf.Bytes()
}

func (m *Writer) Strings() string {
	return m.buf.String()
}

func NewWriter() *Writer {
	m := Writer{}
	return &m
}
