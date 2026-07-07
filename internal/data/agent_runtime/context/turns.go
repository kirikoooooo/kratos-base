package context

import "strings"

func AppendHumanTurnIfNeeded(turns []ConversationTurn, prompt string) []ConversationTurn {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return turns
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role != ConversationRoleHuman {
			continue
		}
		if strings.TrimSpace(turns[i].Content) == prompt {
			return turns
		}
		break
	}
	return append(turns, ConversationTurn{
		Role:    ConversationRoleHuman,
		Content: prompt,
	})
}
