package model

// Event represents a miner event type for notification filtering and logging.
type Event string

// All supported miner events.
const (
	EventStreamerOnline      Event = "STREAMER_ONLINE"
	EventStreamerOffline     Event = "STREAMER_OFFLINE"
	EventGainForRaid        Event = "GAIN_FOR_RAID"
	EventGainForClaim       Event = "GAIN_FOR_CLAIM"
	EventGainForWatch       Event = "GAIN_FOR_WATCH"
	EventGainForWatchStreak Event = "GAIN_FOR_WATCH_STREAK"
	EventBetWin             Event = "BET_WIN"
	EventBetLose            Event = "BET_LOSE"
	EventBetRefund          Event = "BET_REFUND"
	EventBetFilters         Event = "BET_FILTERS"
	EventBetGeneral         Event = "BET_GENERAL"
	EventBetFailed          Event = "BET_FAILED"
	EventBetStart           Event = "BET_START"
	EventBonusClaim         Event = "BONUS_CLAIM"
	EventMomentClaim        Event = "MOMENT_CLAIM"
	EventJoinRaid           Event = "JOIN_RAID"
	EventDropClaim          Event = "DROP_CLAIM"
	EventDropStatus         Event = "DROP_STATUS"
	EventChatMention        Event = "CHAT_MENTION"
	EventTest               Event = "TEST"
)

// AllEvents returns a slice of all defined events.
func AllEvents() []Event {
	return []Event{
		EventStreamerOnline,
		EventStreamerOffline,
		EventGainForRaid,
		EventGainForClaim,
		EventGainForWatch,
		EventGainForWatchStreak,
		EventBetWin,
		EventBetLose,
		EventBetRefund,
		EventBetFilters,
		EventBetGeneral,
		EventBetFailed,
		EventBetStart,
		EventBonusClaim,
		EventMomentClaim,
		EventJoinRaid,
		EventDropClaim,
		EventDropStatus,
		EventChatMention,
		EventTest,
	}
}

// String returns the string representation of an Event.
func (e Event) String() string {
	return string(e)
}

// ParseEvent converts a string to an Event. Returns empty string if invalid.
func ParseEvent(s string) Event {
	for _, e := range AllEvents() {
		if string(e) == s {
			return e
		}
	}
	return ""
}

// Priority defines the watch priority strategy for selecting which streamers to watch.
type Priority int

const (
	// PriorityOrder uses the order defined in the config file.
	PriorityOrder Priority = iota
	// PriorityStreak prioritizes streamers where a watch streak bonus is pending.
	PriorityStreak
	// PriorityDrops prioritizes streamers with active drop campaigns.
	PriorityDrops
	// PrioritySubscribed prioritizes subscribed channels.
	PrioritySubscribed
	// PriorityPointsAscending prioritizes streamers with the fewest points.
	PriorityPointsAscending
	// PriorityPointsDescending prioritizes streamers with the most points.
	PriorityPointsDescending
)

// String returns the string representation of a Priority.
func (p Priority) String() string {
	switch p {
	case PriorityOrder:
		return "ORDER"
	case PriorityStreak:
		return "STREAK"
	case PriorityDrops:
		return "DROPS"
	case PrioritySubscribed:
		return "SUBSCRIBED"
	case PriorityPointsAscending:
		return "POINTS_ASCENDING"
	case PriorityPointsDescending:
		return "POINTS_DESCENDING"
	default:
		return "ORDER"
	}
}

// ParsePriority converts a string to a Priority value.
func ParsePriority(s string) Priority {
	switch s {
	case "ORDER":
		return PriorityOrder
	case "STREAK":
		return PriorityStreak
	case "DROPS":
		return PriorityDrops
	case "SUBSCRIBED":
		return PrioritySubscribed
	case "POINTS_ASCENDING":
		return PriorityPointsAscending
	case "POINTS_DESCENDING":
		return PriorityPointsDescending
	default:
		return PriorityOrder
	}
}

// FollowersOrder defines the sort order for followed channels.
type FollowersOrder int

const (
	// FollowersOrderASC sorts followers in ascending order.
	FollowersOrderASC FollowersOrder = iota
	// FollowersOrderDESC sorts followers in descending order.
	FollowersOrderDESC
)

// String returns the string representation of a FollowersOrder.
func (fo FollowersOrder) String() string {
	switch fo {
	case FollowersOrderASC:
		return "ASC"
	case FollowersOrderDESC:
		return "DESC"
	default:
		return "ASC"
	}
}

// ParseFollowersOrder converts a string to a FollowersOrder value.
func ParseFollowersOrder(s string) FollowersOrder {
	switch s {
	case "ASC":
		return FollowersOrderASC
	case "DESC":
		return FollowersOrderDESC
	default:
		return FollowersOrderASC
	}
}
