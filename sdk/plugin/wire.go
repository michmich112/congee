package plugin

import "github.com/michmich112/congee/sdk/plugin/pluginv1"

func eventToProto(e Event) *pluginv1.Event {
	tags := make([]*pluginv1.Tag, 0, len(e.Tags))
	for _, t := range e.Tags {
		tags = append(tags, &pluginv1.Tag{Values: append([]string(nil), t...)})
	}
	return &pluginv1.Event{
		Id:        e.ID,
		Pubkey:    e.PubKey,
		CreatedAt: e.CreatedAt,
		Kind:      int32(e.Kind),
		Tags:      tags,
		Content:   e.Content,
		Sig:       e.Sig,
	}
}

func eventFromProto(p *pluginv1.Event) Event {
	if p == nil {
		return Event{}
	}
	tags := make([][]string, 0, len(p.Tags))
	for _, t := range p.Tags {
		if t == nil {
			continue
		}
		tags = append(tags, append([]string(nil), t.Values...))
	}
	return Event{
		ID:        p.Id,
		PubKey:    p.Pubkey,
		CreatedAt: p.CreatedAt,
		Kind:      int(p.Kind),
		Tags:      tags,
		Content:   p.Content,
		Sig:       p.Sig,
	}
}

func filterToProto(f Filter) *pluginv1.Filter {
	kinds := make([]int32, len(f.Kinds))
	for i, k := range f.Kinds {
		kinds[i] = int32(k)
	}
	var since, until *int64
	if f.Since != nil {
		v := *f.Since
		since = &v
	}
	if f.Until != nil {
		v := *f.Until
		until = &v
	}
	var limit *int32
	if f.Limit != nil {
		v := int32(*f.Limit)
		limit = &v
	}
	var tfs []*pluginv1.TagFilter
	for name, vals := range f.Tags {
		tfs = append(tfs, &pluginv1.TagFilter{Name: name, Values: append([]string(nil), vals...)})
	}
	return &pluginv1.Filter{
		Ids:        append([]string(nil), f.IDs...),
		Authors:    append([]string(nil), f.Authors...),
		Kinds:      kinds,
		Since:      since,
		Until:      until,
		Limit:      limit,
		Search:     f.Search,
		TagFilters: tfs,
	}
}

func filterFromProto(p *pluginv1.Filter) Filter {
	if p == nil {
		return Filter{Tags: map[string][]string{}}
	}
	kinds := make([]int, len(p.Kinds))
	for i, k := range p.Kinds {
		kinds[i] = int(k)
	}
	var limit *int
	if p.Limit != nil {
		v := int(*p.Limit)
		limit = &v
	}
	tags := make(map[string][]string)
	for _, tf := range p.TagFilters {
		if tf == nil || tf.Name == "" {
			continue
		}
		tags[tf.Name] = append([]string(nil), tf.Values...)
	}
	return Filter{
		IDs:     append([]string(nil), p.Ids...),
		Authors: append([]string(nil), p.Authors...),
		Kinds:   kinds,
		Since:   p.Since,
		Until:   p.Until,
		Limit:   limit,
		Search:  p.Search,
		Tags:    tags,
	}
}

func filtersToProto(in []Filter) []*pluginv1.Filter {
	out := make([]*pluginv1.Filter, 0, len(in))
	for _, f := range in {
		out = append(out, filterToProto(f))
	}
	return out
}

func filtersFromProto(in []*pluginv1.Filter) []Filter {
	out := make([]Filter, 0, len(in))
	for _, p := range in {
		out = append(out, filterFromProto(p))
	}
	return out
}

func subToProto(s TrafficSubscription) *pluginv1.TrafficSubscription {
	kinds := make([]int32, len(s.Kinds))
	for i, k := range s.Kinds {
		kinds[i] = int32(k)
	}
	return &pluginv1.TrafficSubscription{
		MessageTypes:  append([]string(nil), s.MessageTypes...),
		Kinds:         kinds,
		ReqHasSearch:  s.ReqHasSearch,
		ReqTagNames:   append([]string(nil), s.ReqTagNames...),
		InterceptReq:  s.InterceptREQ,
		Observe:       s.Observe,
		OnStoredEvent: s.OnStoredEvent,
	}
}

func subFromProto(p *pluginv1.TrafficSubscription) TrafficSubscription {
	if p == nil {
		return TrafficSubscription{}
	}
	kinds := make([]int, len(p.Kinds))
	for i, k := range p.Kinds {
		kinds[i] = int(k)
	}
	return TrafficSubscription{
		MessageTypes:  append([]string(nil), p.MessageTypes...),
		Kinds:         kinds,
		ReqHasSearch:  p.ReqHasSearch,
		ReqTagNames:   append([]string(nil), p.ReqTagNames...),
		InterceptREQ:  p.InterceptReq,
		Observe:       p.Observe,
		OnStoredEvent: p.OnStoredEvent,
	}
}

func subsToProto(in []TrafficSubscription) []*pluginv1.TrafficSubscription {
	out := make([]*pluginv1.TrafficSubscription, 0, len(in))
	for _, s := range in {
		out = append(out, subToProto(s))
	}
	return out
}

func subsFromProto(in []*pluginv1.TrafficSubscription) []TrafficSubscription {
	out := make([]TrafficSubscription, 0, len(in))
	for _, p := range in {
		out = append(out, subFromProto(p))
	}
	return out
}

func eventsToProto(in []Event) []*pluginv1.Event {
	out := make([]*pluginv1.Event, 0, len(in))
	for _, e := range in {
		out = append(out, eventToProto(e))
	}
	return out
}

func eventsFromProto(in []*pluginv1.Event) []Event {
	out := make([]Event, 0, len(in))
	for _, p := range in {
		out = append(out, eventFromProto(p))
	}
	return out
}
