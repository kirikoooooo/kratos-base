import re
from pathlib import Path

p = Path(r"d:\code\kratos-base\internal\data\agent\langchain.go")
text = p.read_text(encoding="utf-8", errors="replace")

router = '''promptLines := []string{
		"你是 RouterAgent，负责协调本地 Agent 完成任务。",
		"请优先使用可用函数来完成编码、实现、审查等子任务，而不是直接假设工具执行结果。",
		"当任务同时包含开发与审查时，优先调用 coder_agent，再根据结果调用 reviewer_agent。",
		"当你已经拿到足够的工具结果后，再直接输出最终中文结论。",
	}
	systemPrompt := strings.Join(promptLines, "\\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "router agent 已通过本地 tool calling 完成编排")'''

default = '''systemPrompt := strings.Join([]string{
		"你是一个通用代码代理运行时，负责直接理解任务并给出可执行结果。",
		"当任务涉及查看仓库、修改文件或执行本地验证时，优先调用可用函数，不要假设工具已经执行。",
		"可用工具覆盖读文件、edit_file 增量编辑、write_file 新建文件和执行受限验证命令；拿到工具结果后，再用中文给出真实总结。",
		"如果任务不需要工具，也可以直接回答，但不能编造执行结果。",
	}, "\\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "default agent profile 已通过通用 runtime 完成任务")'''

coder = '''promptLines := []string{
		"你是 CoderAgent，负责真实完成实现、编码、原型设计、接口定义与技术方案落地。",
		"请直接基于用户输入给出真实中文结果，不要返回模板化占位文本，不要假设自己已经完成未执行的操作。",
		"当任务更适合先做任务拆解、协调或改由 RouterAgent 统筹时，调用 router_agent。",
		"当你已经产出实现方案且需要补充审查意见时，可以调用 reviewer_agent。",
		"如果任务可以直接回答，就直接输出最终中文结果。",
	}
	systemPrompt := strings.Join(promptLines, "\\n")

	return r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "coder agent 已通过本地 tool calling 完成生成")'''

reviewer = '''return r.runPlainLLMTask(ctx, strings.Join([]string{
		"你是 ReviewerAgent，负责进行代码审查、设计评审、边界条件检查和风险识别。",
		"请直接给出真实中文审查结论、问题清单、风险点和改进建议。",
		"不要返回占位文本，不要说自己尚未开始，直接输出有用内容。",
	}, "\\n"), prompt, "reviewer agent 已通过真实 LLM 完成审查")'''

text = re.sub(
    r"promptLines := \[\]string\{.*?\n\t\}\n\tsystemPrompt := strings\.Join\(promptLines, \"\\n\"\)\n\n\treturn r\.runToolCallingLoop\(ctx, modelClient, toolset, systemPrompt, prompt, \"router agent[^\"]*\"\)",
    router,
    text,
    count=1,
    flags=re.S,
)
text = re.sub(
    r"systemPrompt := strings\.Join\(\[\]string\{.*?\}, \"\\n\"\)\n\n\treturn r\.runToolCallingLoop\(ctx, modelClient, toolset, systemPrompt, prompt, \"default agent profile[^\"]*\"\)",
    default,
    text,
    count=1,
    flags=re.S,
)
text = re.sub(
    r"promptLines := \[\]string\{.*?\n\t\}\n\tsystemPrompt := strings\.Join\(promptLines, \"\\n\"\)\n\n\treturn r\.runToolCallingLoop\(ctx, modelClient, toolset, systemPrompt, prompt, \"coder agent[^\"]*\"\)",
    coder,
    text,
    count=1,
    flags=re.S,
)
text = re.sub(
    r"return r\.runPlainLLMTask\(ctx, strings\.Join\(\[\]string\{.*?\}, \"\\n\"\), prompt, \"reviewer agent[^\"]*\"\)",
    reviewer,
    text,
    count=1,
    flags=re.S,
)

p.write_text(text, encoding="utf-8")
print("patched", p)
