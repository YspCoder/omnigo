package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/YspCoder/omnigo"
)

const (
	requestTimeout   = 90 * time.Second
	maxHistoryRounds = 12
	kickoffMessage   = "请开始初始化流程，并在最后等待我输入“开始游戏”。"
)

const systemPrompt = `
你不是普通对话模型。

你是一个持续运行的 **人生模拟Agent**，名称：

# LifeReloaded Agent Lite

你的任务不是讲故事，而是：

> **模拟一个完整、连续、有因果的人生**

---

# 一、核心身份

你同时扮演三种角色：

1. **叙事者**
   用具有画面感的中文讲述人生

2. **状态管理者**
   持续维护玩家状态（不能遗忘）

3. **因果引擎**
   让每个选择影响未来（不是独立事件）

---

# 二、核心原则（必须遵守）

1. 人生必须连续（不能每轮像新故事）
2. 选择必须产生后果（短期 + 长期）
3. 事件必须基于当前状态生成（不是随机）
4. 人物和经历可以回流（不是一次性NPC）
5. 不要偷懒、简化或模板化

---

# 三、玩家状态（必须持续维护）

每一轮你都要记住：

【玩家状态】

* 性别
* 出生地
* 年龄
* 人生阶段（儿童/青春/成年早期/中期/老年）
* MBTI
* 属性：

  * 魅力
  * 智力
  * 健康
  * 富裕
  * 幸福度

【长期记忆】

* 重要选择
* 重要人物
* 重大事件
* 命运倾向（如：孤独、野心、迟成等）

👉 如果环境不支持存储，你必须在文本中维持这个状态

---

# 四、人生阶段规则

不同阶段必须有不同主题：

* 儿童：家庭、启蒙、依赖、第一次挫败
* 青春：认同、学业、情感、自尊
* 成年早期：选择、机会、爱情、试错
* 成年中期：责任、事业、压力、转折
* 老年：衰老、回望、和解、告别

事件必须符合阶段逻辑

---

# 五、事件生成规则

每一轮事件必须包含：

1. 引子（有画面感）
2. 时间
3. 地点
4. 人物
5. 起因
6. 经过

要求：

* 多用细节（气味/声音/环境）
* 用侧面描写，而不是直接说情绪
* 让事件像真实发生过

---

# 六、选项系统

每次必须提供 5 个选项：

* 3 个普通选项
* 1 个基于“属性”的特殊选项
* 1 个基于“性格（MBTI）”的特殊选项

要求：

* 每个选项必须有真实差异
* 必须有代价或风险
* 不允许“明显正确答案”

---

# 七、结果与更新

玩家选择后，你必须：

1. 描述发生了什么（文学化）
2. 更新属性（可小幅变化）
3. 更新关系或记忆（如重要人物）
4. 记录命运影响（可延迟体现）

原则：

* 不要每次都大起大落
* 小选择也可以改变长期轨迹
* 好选择 ≠ 立刻好结果

---

# 八、年龄推进

每轮推进时间（合理即可）：

* 儿童：+1~3岁
* 青春：+1~2岁
* 成年：+2~5岁
* 中年：+3~7岁
* 老年：+1~5岁

---

# 九、结束条件

当发生以下情况：

* 健康/幸福/财富极低
* 或自然老去

→ 游戏结束

必须输出：

* 人生总结
* 命运主线
* 一段有文学性的墓志铭

---

# 十、输出结构（每轮固定）

## 当前事件

（完整叙事）

## 选项

1.
2.
3.

4.（属性特殊）
5.（性格特殊）

## 提示

人无法两次踏入相同的河流，每一个选择都在塑造你的人生。请谨慎选择。

👉 等待玩家输入

---

# 十一、启动流程

第一次运行时：

1. 随机生成：

   * 性别
   * 中国城市
   * 年龄（5~10）
   * 五维属性（1~10）
   * MBTI

2. 写一段“家庭背景故事”（小说化，有细节）

3. 展示属性

4. 写一首短诗（作为人生序章）

5. 提示玩家输入：
   👉 开始游戏

---

# 十二、最终约束（最重要）

你必须始终记住：

你不是在“生成内容”，
你是在：

> **推进一段不可重复的人生**

现在，开始初始化流程。
`

func main() {
	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		log.Fatal("ARK_API_KEY is not set")
	}

	llm, err := omnigo.NewLLM(
		omnigo.SetProvider("ark"),
		omnigo.SetModel("doubao-seed-2-0-mini-260215"),
		omnigo.SetAPIKey(apiKey),
		omnigo.SetEndpoint("https://ark.cn-beijing.volces.com/api/v3"),
		omnigo.SetMaxTokens(4096),
	)
	if err != nil {
		log.Fatalf("create llm failed: %v", err)
	}

	history := []omnigo.PromptMessage{
		{Role: "system", Content: systemPrompt},
	}

	fmt.Println("助手: ")
	answer, err := streamReply(context.Background(), llm, append(history, omnigo.PromptMessage{
		Role:    "user",
		Content: kickoffMessage,
	}))
	if err != nil {
		log.Fatalf("initialization failed: %v", err)
	}
	fmt.Println()

	history = append(history,
		omnigo.PromptMessage{Role: "user", Content: kickoffMessage},
		omnigo.PromptMessage{Role: "assistant", Content: answer},
	)

	fmt.Println("\n输入内容继续游戏，输入 `reset` 重开，输入 `exit` 退出。")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Print("\n你: ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				log.Fatalf("read input failed: %v", err)
			}
			break
		}

		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		switch userInput {
		case "exit", "quit":
			return
		case "reset":
			history = []omnigo.PromptMessage{{Role: "system", Content: systemPrompt}}
			fmt.Println("助手: ")
			answer, err = streamReply(context.Background(), llm, append(history, omnigo.PromptMessage{
				Role:    "user",
				Content: kickoffMessage,
			}))
			if err != nil {
				log.Fatalf("reset failed: %v", err)
			}
			fmt.Println()
			history = append(history,
				omnigo.PromptMessage{Role: "user", Content: kickoffMessage},
				omnigo.PromptMessage{Role: "assistant", Content: answer},
			)
			continue
		}

		history = append(history, omnigo.PromptMessage{
			Role:    "user",
			Content: userInput,
		})
		history = trimHistory(history, maxHistoryRounds)

		fmt.Print("助手: ")
		answer, err = streamReply(context.Background(), llm, history)
		if err != nil {
			log.Fatalf("stream failed: %v", err)
		}
		fmt.Println()

		history = append(history, omnigo.PromptMessage{
			Role:    "assistant",
			Content: answer,
		})
	}
}

func streamReply(parent context.Context, llm omnigo.LLM, history []omnigo.PromptMessage) (string, error) {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()

	prompt := omnigo.NewPrompt("", omnigo.WithMessages(history))
	stream, err := llm.Stream(ctx, prompt)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var answer strings.Builder
	for {
		token, err := stream.Next(ctx)
		if err != nil {
			if err == io.EOF {
				return answer.String(), nil
			}
			return "", err
		}
		fmt.Print(token.Text)
		answer.WriteString(token.Text)
	}
}

func trimHistory(history []omnigo.PromptMessage, maxRounds int) []omnigo.PromptMessage {
	if len(history) == 0 {
		return history
	}

	keep := 1 + maxRounds*2
	if len(history) <= keep {
		return history
	}

	trimmed := make([]omnigo.PromptMessage, 0, keep)
	trimmed = append(trimmed, history[0])
	trimmed = append(trimmed, history[len(history)-(keep-1):]...)
	return trimmed
}
