package rt

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// xmlValue 是 XML-RPC <value> 节点的递归表示。
// Array 指针非空 → 该 value 包裹一个数组；否则取 String/I4/I8/Double/Chardata 作为标量文本。
// rTorrent 的 system.multicall 响应里，每个被调命令的结果都被裹成 array，
// 故需要递归结构而非 encoding/xml 的扁平映射。
type xmlValue struct {
	String   string    `xml:"string"`
	I4       string    `xml:"i4"`
	I8       string    `xml:"i8"`
	Double   string    `xml:"double"`
	Chardata string    `xml:",chardata"`
	Array    *xmlArray `xml:"array"`
}

type xmlArray struct {
	Data struct {
		Values []xmlValue `xml:"value"`
	} `xml:"data"`
}

// text 取该 value 的标量字符串形式（按 rTorrent 实际返回的类型优先级）。
func (v xmlValue) text() string {
	if v.String != "" {
		return v.String
	}
	if v.I4 != "" {
		return v.I4
	}
	if v.I8 != "" {
		return v.I8
	}
	if v.Double != "" {
		return v.Double
	}
	return strings.TrimSpace(v.Chardata)
}

// parseMulticallResults 解析 system.multicall 的 XML-RPC 响应，返回每个被调命令首个返回值的字符串形式。
// 响应结构：
//
//	methodResponse > params > param > value > array   (外层 multicall 结果数组)
//	  > value > array > data > value                  (每个命令的结果数组，其首个 value 是实际返回值)
//
// 用于 GetSpeedLimits（get_upload_rate / get_download_rate 返回单值）。
// 写类 multicall（recheck/priority 等）只校验 HTTP 状态，不走此解析。
func parseMulticallResults(body []byte) ([]string, error) {
	var resp struct {
		Params []struct {
			Param struct {
				Value xmlValue `xml:"value"`
			} `xml:"param"`
		} `xml:"params"`
	}
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse multicall xml: %w", err)
	}
	if len(resp.Params) == 0 || resp.Params[0].Param.Value.Array == nil {
		return nil, fmt.Errorf("empty multicall response")
	}
	top := resp.Params[0].Param.Value.Array.Data.Values
	out := make([]string, 0, len(top))
	for _, cmdRes := range top {
		// 每个命令结果本身是一个 array；取其首个 value 为实际返回值
		if cmdRes.Array == nil || len(cmdRes.Array.Data.Values) == 0 {
			out = append(out, "")
			continue
		}
		out = append(out, cmdRes.Array.Data.Values[0].text())
	}
	return out, nil
}
