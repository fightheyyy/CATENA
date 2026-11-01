import type { ConversationContentPart, ConversationMessage, ConversationSummary } from "./types";

export function conversationKey(conversation: Pick<ConversationSummary, "agent_id" | "conversation_id">) {
  return `${conversation.agent_id}\u0000${conversation.conversation_id}`;
}

export function conversationMessageText(parts: ConversationContentPart[]) {
  return parts
    .map((part) => part.type === "text" ? part.text?.trim() ?? "" : part.name ? `[${part.name}]` : "")
    .filter(Boolean)
    .join("\n");
}

export function orderedConversationMessages(messages: ConversationMessage[]) {
  return [...messages].sort((left, right) => (
    left.sequence - right.sequence || left.occurred_at.localeCompare(right.occurred_at)
  ));
}
