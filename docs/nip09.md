# NIP-09 deletion requests

Congee stores signed kind `5` deletion requests and enforces their `e` and `a` tags for events by the same author. It keeps the request event available to subscribers. An `e` tag removes the matching event ID; an `a` tag removes matching replaceable or addressable revisions created no later than the request. A later revision of an addressable event may be stored.

The event database schema migration to version 8 creates durable ID and address tombstones. A covered event imported later through NIP-77 or submitted by a client is rejected instead of resurrected. Deletion requests with invalid signatures are rejected. Unrecognized tags are ignored; a request cannot delete another author's events or undo another deletion request.

As with NIP-09 generally, deletion on this relay cannot remove copies already held by other relays or clients.
