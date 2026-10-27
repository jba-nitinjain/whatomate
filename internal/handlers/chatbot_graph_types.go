package handlers

import (
	"github.com/nikyjain/whatomate/internal/flowgraph"
)

// ChatNodeType identifies the kind of node in a chatbot flow graph.
// Constants live in this package so the chat domain owns its vocabulary
// without coupling to internal/calling.
type ChatNodeType string

const (
	ChatNodeStart        ChatNodeType = "start"
	ChatNodeMessage      ChatNodeType = "message"
	ChatNodeButtons      ChatNodeType = "buttons"
	ChatNodePrompt       ChatNodeType = "prompt"
	ChatNodeAPICall      ChatNodeType = "api_call"
	ChatNodeCondition    ChatNodeType = "condition"
	ChatNodeTiming       ChatNodeType = "timing"
	ChatNodeSetVariable  ChatNodeType = "set_variable"
	ChatNodeAIResponse   ChatNodeType = "ai_response"
	ChatNodeTransfer     ChatNodeType = "transfer"
	ChatNodeWebhook      ChatNodeType = "webhook"
	ChatNodeGotoFlow     ChatNodeType = "goto_flow"
	ChatNodeWhatsAppFlow ChatNodeType = "whatsapp_flow"
	ChatNodeEnd          ChatNodeType = "end"
)

// ChatNode, ChatEdge and ChatGraph are the chatbot domain's views of the shared
// flow-graph types, specialized to ChatNodeType. Edge conditions for the chat
// engine include "default", "button:<id>", "input:<val>", "http:2xx",
// "http:non2xx", "validation_failed", "max_retries", "in_hours", "out_of_hours".
// Traversal (BuildMaps/Node/ResolveEdge) lives in internal/flowgraph.
type (
	ChatNode  = flowgraph.Node[ChatNodeType]
	ChatEdge  = flowgraph.Edge
	ChatGraph = flowgraph.Graph[ChatNodeType]
)
