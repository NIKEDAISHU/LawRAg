package splitter

import (
	"testing"
)

func TestDiscretionSplitter_Split(t *testing.T) {
	splitter := NewDiscretionSplitter()

	content := `【湖北省裁量基准】未取得《医疗机构执业许可证》擅自执业的
【违法行为】：未取得《医疗机构执业许可证》擅自执业的
【法律依据】：《中华人民共和国基本医疗卫生与健康促进法》第九十九条第一款　违反本法规定，未取得医疗机构执业许可证擅自执业的，由县级以上人民政府卫生健康主管部门责令停止执业活动，没收违法所得和药品、医疗器械，并处违法所得五倍以上二十倍以下的罚款，违法所得不足一万元的，按一万元计算。　
【违法程度】：轻微
【适用情形】：1.首次发现，擅自执业时间不足3个月，及时纠正并未给患者造成人身伤害的；2.擅自执业人员为卫生专业技术人员的；3.违法所得不足1万元的；4.未经批准在登记的执业地点以外开展诊疗活动的。
【处罚标准】：没收违法所得和药品、医疗器械，并处违法所得5倍以上10倍以下罚款。违法所得不足1万元的，按1万元计算。

======

【湖北省裁量基准】未取得《医疗机构执业许可证》擅自执业的
【违法行为】：未取得《医疗机构执业许可证》擅自执业的
【法律依据】：《中华人民共和国基本医疗卫生与健康促进法》第九十九条第一款　违反本法规定，未取得医疗机构执业许可证擅自执业的，由县级以上人民政府卫生健康主管部门责令停止执业活动，没收违法所得和药品、医疗器械，并处违法所得五倍以上二十倍以下的罚款，违法所得不足一万元的，按一万元计算。　
【违法程度】：一般
【适用情形】：1.再次发现的；2.擅自执业时间3个月以上不足6个月的；3.违法所得1万元以上不足3万元的；4.使用1名非卫生技术专业人员执业的；5.给患者造成局部组织、器官结构的轻微损害或轻度短暂功能障碍的损伤的。
【处罚标准】：没收违法所得和药品、医疗器械，并处违法所得10倍以上15倍以下罚款。违法所得不足1万元的，按1万元计算。

======`

	chunks := splitter.Split(content)

	if len(chunks) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(chunks))
	}

	// 检查第一个 chunk
	if chunks[0].Metadata["region"] != "湖北省" {
		t.Errorf("Expected region '湖北省', got '%v'", chunks[0].Metadata["region"])
	}

	if chunks[0].Metadata["violation"] != "未取得《医疗机构执业许可证》擅自执业的" {
		t.Errorf("Expected violation '未取得《医疗机构执业许可证》擅自执业的', got '%v'", chunks[0].Metadata["violation"])
	}

	if chunks[0].Metadata["severity"] != "轻微" {
		t.Errorf("Expected severity '轻微', got '%v'", chunks[0].Metadata["severity"])
	}

	if chunks[0].Metadata["law_name"] != "中华人民共和国基本医疗卫生与健康促进法" {
		t.Errorf("Expected law_name '中华人民共和国基本医疗卫生与健康促进法', got '%v'", chunks[0].Metadata["law_name"])
	}

	// 检查第二个 chunk
	if chunks[1].Metadata["severity"] != "一般" {
		t.Errorf("Expected severity '一般', got '%v'", chunks[1].Metadata["severity"])
	}
}
