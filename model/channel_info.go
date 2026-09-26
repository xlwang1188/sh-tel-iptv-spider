package model

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelInfo struct {
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
	TsTime        int            `gorm:"comment:TimeShiftTime 时移时间" json:"tsTime"`
	Code          string         `gorm:"comment:频道代码" json:"code"`
	AuthCode      string         `gorm:"comment:付费认证代码" json:"authCode"`
	Name          string         `gorm:"comment:频道名称" json:"name"`
	ChID          string         `gorm:"comment:频道ID" json:"ID"`
	MixNo         string         `gorm:"primarykey;comment:用户频道映射" json:"mixNo"`
	MediaID       string         `gorm:"comment:未知" json:"mediaID"`
	IsTs          string         `gorm:"comment:是否支持回放" json:"isTs"`
	IsCharge      string         `gorm:"comment:是否需要付费" json:"isCharge"`
	IsHD          bool           `gorm:"default:false;comment:是否是高清频道" json:"isHD"`
	Is4K          bool           `gorm:"default:false;comment:是否是4K频道" json:"is4K"`
	IsPullEPG     bool           `gorm:"default:true;comment:是否拉取节目单" json:"isPullEPG"`
	IsShow        bool           `gorm:"default:true;comment:是否展示该节目" json:"isShow"`
	CommName      string         `gorm:"comment:通用标题" json:"CommName"`
	LastFetchTime time.Time      `gorm:"comment:节目单最后更新时间" json:"LastFetchTime"`
}

func (h *ChannelInfo) ProcessData() {
    name := strings.ToUpper(h.Name)
    h.CommName = name

    // --- 4K/HD 识别逻辑 ---
    keywords4K := []string{"「4K」", "[4K]", "(4K)", "4K"}
    for _, kw := range keywords4K {
        if strings.Contains(name, kw) {
            h.IsHD = true
            h.Is4K = true
            h.CommName = strings.TrimSpace(strings.ReplaceAll(h.CommName, kw, ""))
            break
        }
    }
    if strings.Contains(name, "HD") || strings.Contains(name, "高清") {
        h.IsHD = true
        h.CommName = strings.TrimSpace(strings.ReplaceAll(h.CommName, "HD", ""))
        h.CommName = strings.TrimSpace(strings.ReplaceAll(h.CommName, "高清", ""))
    }

    // --- 核心修复：Logo 自动关联逻辑 ---
    // 针对 CCTV 做特殊处理，例如 CCTV1 -> CCTV-1
    logoName := h.CommName
    if strings.HasPrefix(logoName, "CCTV") && !strings.Contains(logoName, "-") {
        numPart := strings.TrimPrefix(logoName, "CCTV")
        if numPart != "" {
            logoName = "CCTV-" + numPart
        }
    }
}

func (h *ChannelInfo) updateMapping(tx *gorm.DB) {
	if h.CommName == "" {
		return
	}
	var groups []string
	// 根据 CommName 或 Name 设置分组
	if strings.Contains(h.CommName, "CCTV") {
		groups = append(groups, "央视")
	} else if strings.Contains(h.Name, "卫视") {
		groups = append(groups, "卫视")
	} else if strings.Contains(h.Name, "购物") {
		groups = append(groups, "购物")
	} else if strings.Contains(h.Name, "年级") {
		groups = append(groups, "空中课堂")
	} else if strings.Contains(h.Name, "百事通") {
		groups = append(groups, "百事通")
	} else if strings.Contains(h.Name, "影") {
		groups = append(groups, "电影")
	} else {
		// 如果没有匹配到任何分组，归为 "其他"
		groups = append(groups, "其他")
	}

	tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "comm_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"auto_groups"}),
	}).Create(&M3u8Mapping{
		CommName:   h.CommName,
		AutoGroups: strings.Join(groups, ","),
	})
}
func (h *ChannelInfo) BeforeCreate(tx *gorm.DB) (err error) {
	h.ProcessData()
	h.updateMapping(tx)
	return
}

func (h *ChannelInfo) BeforeUpdate(tx *gorm.DB) (err error) {
	h.ProcessData()
	h.updateMapping(tx)
	return
}

// RemoveDuplicateChannelInfo  ChannelInfo 数组去重
func RemoveDuplicateChannelInfo(in []ChannelInfo) []ChannelInfo {
	newMap := make(map[string]ChannelInfo, len(in))
	for _, child := range in {
		if ch, ok := newMap[child.CommName]; ok {
			// 打印出重复的 CommName 和替换前后的信息
			//fmt.Printf("去重: %s, 旧数据: %+v, 新数据: %+v\n", child.CommName, ch, child)
			// 判断能否替换
			newMap[child.CommName] = check(ch, child)
			continue
		}
		newMap[child.CommName] = child
	}
	var newArr []ChannelInfo
	for _, v := range newMap {
		newArr = append(newArr, v)
	}
	// sort by mix_no
	sort.Slice(newArr, func(i, j int) bool {
		// 尝试数字比较（如果都是数字）
		numi, errI := strconv.Atoi(newArr[i].MixNo)
		numj, errJ := strconv.Atoi(newArr[j].MixNo)
		if errI == nil && errJ == nil {
			return numi < numj
		}
		
		// 使用字符串比较
		// 使用 strings.Compare 进行更准确的字符串比较
		return strings.Compare(newArr[i].MixNo, newArr[j].MixNo) < 0
	})
	return newArr
}

func check(c1, c2 ChannelInfo) ChannelInfo {
	if c1.Is4K {
		return c1
	} else if c2.Is4K {
		return c2
	} else if c1.IsHD {
		return c1
	}
	return c2
}
