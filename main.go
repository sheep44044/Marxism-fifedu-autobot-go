package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	BaseURL  = "https://adapt.fifedu.com/adapt-web"
	Cookie   = "your-cookie-id"                   // 👈 【必须修改】填入你的完整 Cookie
	CourseID = "56e639df41224bc1b40b64690bab4a87" // 课程 ID，通常不用动
)

var client = &http.Client{Timeout: 15 * time.Second}

// 数据结构定义
type ChapterNode struct {
	ChapterID   string        `json:"chapterId"`
	ChapterName string        `json:"chapterName"`
	Children    []ChapterNode `json:"children"`
}

type ChapterListResp struct {
	Data struct {
		CourseChapterDtoList []ChapterNode `json:"courseChapterDtoList"`
	} `json:"data"`
}

type ActivityItem struct {
	ActivityID       string `json:"activityId"`
	ActivityName     string `json:"activityName"`
	CompletionStatus int    `json:"completionStatus"`
}

type ActivityListResp struct {
	Data []ActivityItem `json:"data"`
}

type BatchNoResp struct {
	Data string `json:"data"`
}

type NextResponse struct {
	Data struct {
		QuestionID   string        `json:"questionId"`
		AnswerArray  []interface{} `json:"answerArray"`
		QuestionType struct {
			Name string `json:"name"`
		} `json:"questionType"`
	} `json:"data"`
}

type SubmitPayload struct {
	ActivityID     string         `json:"activityId"`
	CourseID       string         `json:"courseId"`
	BatchNo        string         `json:"batchNo"`
	UserAnswerJson AnswerJsonData `json:"userAnswerJson"`
}

type AnswerJsonData struct {
	QuestionID string `json:"questionId"`
	UserAnswer []int  `json:"userAnswer"`
}

type CheckResponse struct {
	Data struct {
		Status         int    `json:"status"`
		NextQuestionId string `json:"nextQuestionId"`
	} `json:"data"`
}

// ==========================================
// 🏃 主逻辑入口
// ==========================================
func main() {
	fmt.Println("=========================================")
	fmt.Println("马克思自动刷题引擎 (Go 高速版)")
	fmt.Println("=========================================")

	// 1. 获取全书章节目录
	fmt.Println("📚 正在获取全书章节目录...")
	chapters, err := getChapterTree()
	if err != nil {
		fmt.Printf("❌ 获取章节失败，请检查 Cookie 是否过期: %v\n", err)
		return
	}

	leaves := findLeaves(chapters)
	fmt.Printf("✅ 成功解析出 %d 个包含练习的底层小节。\n", len(leaves))

	// 2. 遍历每个小节
	for _, chapter := range leaves {
		fmt.Printf("\n=========================================\n")
		fmt.Printf("🎯 正在进入小节: %s\n", chapter.ChapterName)

		activities, err := getActivities(chapter.ChapterID)
		if err != nil {
			fmt.Printf("❌ 获取活动列表失败: %v\n", err)
			continue
		}

		if len(activities) == 0 {
			fmt.Println("   ⚠️ 该小节下无练习活动，跳过。")
			continue
		}

		// 3. 遍历小节内的活动（练习）
		for _, act := range activities {
			fmt.Printf("\n   🎮 开始练习活动: %s\n", act.ActivityName)

			if act.CompletionStatus == 1 {
				fmt.Println("   ⏭️ 检测到已完成，跳过。")
				continue
			}

			batchNo, err := getBatchNo(act.ActivityID)
			if err != nil || batchNo == "" {
				fmt.Printf("   ❌ 获取入场券(BatchNo)失败: %v\n", err)
				continue
			}
			fmt.Printf("   🎫 获取入场券成功: %s\n", batchNo)

			// 4. 开始单章光速答题循环
			solveActivity(act.ActivityID, batchNo)
		}
	}
	fmt.Println("\n🎉🎉🎉 全书遍历答题完成！你已经是满分学霸了！")
}

// 获取整本书的章节树
func getChapterTree() ([]ChapterNode, error) {
	url := fmt.Sprintf("%s/courseApi/newCourseChapterList?courseId=%s", BaseURL, CourseID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Cookie", Cookie)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res ChapterListResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Data.CourseChapterDtoList, nil
}

// 递归找寻叶子节点（最底层的真正包含题目的小节）
func findLeaves(nodes []ChapterNode) []ChapterNode {
	var leaves []ChapterNode
	for _, n := range nodes {
		if len(n.Children) > 0 {
			leaves = append(leaves, findLeaves(n.Children)...)
		} else {
			leaves = append(leaves, n)
		}
	}
	return leaves
}

// 获取某个小节下的练习列表
func getActivities(chapterId string) ([]ActivityItem, error) {
	url := fmt.Sprintf("%s/courseApi/getActivityListByChapter?courseId=%s&chapterId=%s&type=1", BaseURL, CourseID, chapterId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Cookie", Cookie)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res ActivityListResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

// 获取单次练习的批次号（入场券）
func getBatchNo(activityId string) (string, error) {
	url := fmt.Sprintf("%s/activityApi/getBatchNo?courseId=%s&activityId=%s&vertexId=", BaseURL, CourseID, activityId)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Cookie", Cookie)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res BatchNoResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Data, nil
}

func solveActivity(activityId, batchNo string) {
	currentQuestionId := ""
	questionIndex := 1

	for {
		answers, nextIdFromGet, err := fetchQuestionAndAnswer(activityId, batchNo, currentQuestionId)
		if err != nil {
			fmt.Printf("      ❌ 获取第 %d 题失败: %v\n", questionIndex, err)
			break
		}

		if currentQuestionId == "" {
			currentQuestionId = nextIdFromGet
		}

		if currentQuestionId == "" {
			fmt.Println("      ✅ 该活动没有题目或已完成！")
			break
		}

		nextQuestionId, err := submitAnswer(activityId, batchNo, currentQuestionId, answers)
		if err != nil {
			fmt.Printf("      ❌ 提交第 %d 题失败: %v\n", questionIndex, err)
			break
		}

		// 安全截取显示 ID，防止短 ID 导致越界崩溃
		displayId := currentQuestionId
		if len(currentQuestionId) > 8 {
			displayId = currentQuestionId[:8] + "..."
		}
		fmt.Printf("      ✔ 第 %d 题 [%s] 秒杀成功，自动选了: %v\n", questionIndex, displayId, answers)

		if nextQuestionId == "" {
			fmt.Println("      ✅ 顺利到达最后一题，本练习完美结束！")
			break
		}
		currentQuestionId = nextQuestionId
		questionIndex++
	}
}

// 获取题目并直接从 JSON 中剥离正确答案
func fetchQuestionAndAnswer(activityId, batchNo, qId string) ([]int, string, error) {
	url := fmt.Sprintf("%s/questionApi/next?activityId=%s&courseId=%s&batchNo=%s&questionId=%s&ifForce=0&type=2",
		BaseURL, activityId, CourseID, batchNo, qId)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Cookie", Cookie)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var nextResp NextResponse
	if err := json.Unmarshal(body, &nextResp); err != nil {
		return nil, "", fmt.Errorf("解析 JSON 失败: %v", err)
	}

	var correctAnswers []int
	for _, val := range nextResp.Data.AnswerArray {
		switch v := val.(type) {
		case float64:
			correctAnswers = append(correctAnswers, int(v))
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				correctAnswers = append(correctAnswers, parsed)
			}
		}
	}

	if len(correctAnswers) == 0 {
		correctAnswers = []int{0}
	}

	return correctAnswers, nextResp.Data.QuestionID, nil
}

// 提交正确答案并拉取下一题的 ID
func submitAnswer(activityId, batchNo, qId string, answers []int) (string, error) {
	checkURL := fmt.Sprintf("%s/questionApi/check", BaseURL)

	payload := SubmitPayload{
		ActivityID: activityId,
		CourseID:   CourseID,
		BatchNo:    batchNo,
		UserAnswerJson: AnswerJsonData{
			QuestionID: qId,
			UserAnswer: answers,
		},
	}

	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", checkURL, bytes.NewBuffer(jsonData))
	req.Header.Add("Cookie", Cookie)
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var checkResp CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return "", err
	}

	return checkResp.Data.NextQuestionId, nil
}
