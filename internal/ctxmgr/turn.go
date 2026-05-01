package ctxmgr

import "github.com/anthropics/anthropic-sdk-go"

type TurnPriority int

const (
	PriorityCore          TurnPriority = 1
	PriorityResearch      TurnPriority = 2
	PrioritySupplementary TurnPriority = 3
)

type Turn struct {
	Index           int
	Assistant       anthropic.MessageParam
	ToolResults     *anthropic.MessageParam
	EstimatedTokens int
	Compacted       bool
	CompactionLevel int
	Priority        TurnPriority
}

type MessageLog struct {
	initialUserMessage anthropic.MessageParam
	turns              []Turn
}

func NewMessageLog(initialMessage anthropic.MessageParam) *MessageLog {
	return &MessageLog{
		initialUserMessage: initialMessage,
	}
}

func (ml *MessageLog) AddTurn(assistant anthropic.MessageParam, toolResults *anthropic.MessageParam, estimatedTokens int, priority TurnPriority) {
	ml.turns = append(ml.turns, Turn{
		Index:           len(ml.turns),
		Assistant:       assistant,
		ToolResults:     toolResults,
		EstimatedTokens: estimatedTokens,
		Priority:        priority,
	})
}

func (ml *MessageLog) GetTurn(index int) *Turn {
	if index < 0 || index >= len(ml.turns) {
		return nil
	}
	return &ml.turns[index]
}

func (ml *MessageLog) Len() int {
	return len(ml.turns)
}

func (ml *MessageLog) BuildMessages() []anthropic.MessageParam {
	msgs := []anthropic.MessageParam{ml.initialUserMessage}
	for _, turn := range ml.turns {
		msgs = append(msgs, turn.Assistant)
		if turn.ToolResults != nil {
			msgs = append(msgs, *turn.ToolResults)
		}
	}
	return msgs
}
