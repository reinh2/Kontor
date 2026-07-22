package tools

import "encoding/json"

const schemaPrefix = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object",`

var definitions = []Definition{
	{
		Name: ToolListServices, Version: ContractVersion,
		Description: "List active services with duration, buffers, and price.",
		Parameters:  schema(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","maxProperties":0,"additionalProperties":false}`),
	},
	{
		Name: ToolListStaff, Version: ContractVersion,
		Description: "List staff who can perform a service.",
		Parameters:  schema(schemaPrefix + `"required":["service_id"],"properties":{"service_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
	},
	{
		Name: ToolFindSlots, Version: ContractVersion,
		Description: "Find currently free appointment slots. Returned slots are not held until booking succeeds.",
		Parameters:  schema(schemaPrefix + `"required":["service_id","date_from","date_to"],"properties":{"service_id":{"type":"string","format":"uuid"},"staff_id":{"type":"string","format":"uuid"},"date_from":{"type":"string","format":"date-time","pattern":"(Z|[+-][0-9]{2}:[0-9]{2})$"},"date_to":{"type":"string","format":"date-time","pattern":"(Z|[+-][0-9]{2}:[0-9]{2})$"}},"additionalProperties":false}`),
	},
	{
		Name: ToolCreateBooking, Version: ContractVersion,
		Description: "Propose or, after a server-authorized confirmation, create a booking using a slot token.",
		Parameters:  schema(schemaPrefix + `"required":["slot_token","customer","idempotency_key"],"properties":{"slot_token":{"type":"string","minLength":32,"maxLength":1024,"pattern":"^slt_v1_[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+$"},"customer":{"type":"object","required":["display_name","contact"],"properties":{"display_name":{"type":"string","minLength":1,"maxLength":200},"contact":{"type":"object","properties":{"email":{"type":"string","format":"email","maxLength":254},"phone":{"type":"string","pattern":"^\\+[1-9][0-9]{7,14}$"}},"anyOf":[{"required":["email"]},{"required":["phone"]}],"additionalProperties":false},"company":{"type":"string","maxLength":200},"locale":{"type":"string","minLength":2,"maxLength":35}},"additionalProperties":false},"notes":{"type":"string","maxLength":500},"idempotency_key":{"type":"string","minLength":16,"maxLength":128,"pattern":"^[A-Za-z0-9][A-Za-z0-9._:-]*$"},"confirmation_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
	},
	{
		Name: ToolReschedule, Version: ContractVersion,
		Description: "Propose or, after confirmation, reschedule an owned booking.",
		Parameters:  schema(schemaPrefix + `"required":["booking_id","new_slot","idempotency_key"],"properties":{"booking_id":{"type":"string","format":"uuid"},"new_slot":{"type":"object","required":["slot_token"],"properties":{"slot_token":{"type":"string","minLength":32,"maxLength":1024,"pattern":"^slt_v1_[A-Za-z0-9_-]+\\.[A-Za-z0-9_-]+$"}},"additionalProperties":false},"idempotency_key":{"type":"string","minLength":16,"maxLength":128,"pattern":"^[A-Za-z0-9][A-Za-z0-9._:-]*$"},"confirmation_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
	},
	{
		Name: ToolCancel, Version: ContractVersion,
		Description: "Propose or, after confirmation, cancel an owned booking.",
		Parameters:  schema(schemaPrefix + `"required":["booking_id","reason","idempotency_key"],"properties":{"booking_id":{"type":"string","format":"uuid"},"reason":{"type":"string","minLength":1,"maxLength":500},"idempotency_key":{"type":"string","minLength":16,"maxLength":128,"pattern":"^[A-Za-z0-9][A-Za-z0-9._:-]*$"},"confirmation_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
	},
	{
		Name: ToolUpsertContact, Version: ContractVersion,
		Description: "Create or update the authenticated customer's CRM contact.",
		Parameters:  schema(schemaPrefix + `"required":["profile","idempotency_key"],"properties":{"profile":{"type":"object","required":["display_name","contact"],"properties":{"display_name":{"type":"string","minLength":1,"maxLength":200},"contact":{"type":"object","properties":{"email":{"type":"string","format":"email","maxLength":254},"phone":{"type":"string","pattern":"^\\+[1-9][0-9]{7,14}$"}},"anyOf":[{"required":["email"]},{"required":["phone"]}],"additionalProperties":false},"company":{"type":"string","maxLength":200},"locale":{"type":"string","minLength":2,"maxLength":35}},"additionalProperties":false},"idempotency_key":{"type":"string","minLength":16,"maxLength":128,"pattern":"^[A-Za-z0-9][A-Za-z0-9._:-]*$"}},"additionalProperties":false}`),
	},
	{
		Name: ToolCreateDeal, Version: ContractVersion,
		Description: "Create a CRM deal for an owned confirmed booking when a server-issued workflow grant permits it.",
		Parameters:  schema(schemaPrefix + `"required":["booking_id","contact_ref","idempotency_key"],"properties":{"booking_id":{"type":"string","format":"uuid"},"contact_ref":{"type":"string","minLength":32,"maxLength":600,"pattern":"^ctr_v1_[A-Za-z0-9_-]+$"},"idempotency_key":{"type":"string","minLength":16,"maxLength":128,"pattern":"^[A-Za-z0-9][A-Za-z0-9._:-]*$"}},"additionalProperties":false}`),
	},
	{
		Name: ToolEscalate, Version: ContractVersion,
		Description: "Hand this conversation to a human operator.",
		Parameters:  schema(schemaPrefix + `"required":["reason"],"properties":{"reason":{"type":"object","required":["code","summary"],"properties":{"code":{"type":"string","enum":["customer_request","understanding_failed","tool_refused","provider_failure","policy","other"]},"summary":{"type":"string","minLength":1,"maxLength":1000}},"additionalProperties":false}},"additionalProperties":false}`),
	},
	{
		Name: ToolRespondToCustomer, Version: ContractVersion,
		Description: "Mandatory terminal control call for a customer-facing reply. Call it alone, never alongside another tool. Use clarification_needed only when missing information prevents answering or acting; the server escalates after three consecutive clarification outcomes.",
		Parameters:  schema(schemaPrefix + `"required":["disposition","message"],"properties":{"disposition":{"type":"string","enum":["complete","clarification_needed"]},"message":{"type":"string","minLength":1,"maxLength":2000,"pattern":"\\S"}},"additionalProperties":false}`),
	},
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

// Definitions returns a defensive copy of the exact v1 allowlist.
func Definitions() []Definition {
	out := make([]Definition, len(definitions))
	for i, definition := range definitions {
		out[i] = definition
		out[i].Parameters = append(json.RawMessage(nil), definition.Parameters...)
	}
	return out
}
