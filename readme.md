# TracimPushNotification

A push notification service for Tracim hooked on [TracimDaemon](https://github.com/Millefeuille42/TracimDaemon) events

## Features

- Push notification on Tracim events
- Config-based notification crafting and filtering

It uses [Gotify](https://gotify.net/) as a notification server.

## Configuration

### Base configuration

TracimPushNotification is configured via environment variables.

- `TRACIM_PUSH_NOTIFICATION_CONFIG`: Path to the config folder (all files will be parsed)
- `TRACIM_PUSH_NOTIFICATION_SOCKET`: Path to the socket file
- `TRACIM_PUSH_NOTIFICATION_MASTER_SOCKET`: Path to the master socket file
- `TRACIM_PUSH_NOTIFICATION_GOTIFY_URL`: URL of the Gotify server

### Notification configuration

The notification configuration is stored in a JSON file.

The file is structured as follows:

```json
[
  {
    "name": "reaction_created_comment",
    "event_type": "reaction.created",
    "elements": [
      {
        "name": "reaction_author_username",
        "key": "reaction.author.username"
      },
      {
        "name": "reaction_value",
        "key": "reaction.value"
      },
      {
        "name": "comment_name",
        "key": "content.parent_label"
      }
    ],
    "filters" : [
      {
        "name": "content_type",
        "key": "content.content_type",
        "match": "equal",
        "value": "comment"
      },
      {
        "name": "users_content",
        "key": "content.author.username",
        "match": "equal",
        "value": "millefeuille"
      }
    ],
    "notification": {
      "title": "Tracim reaction ({{reaction_author_username}})",
      "message": "{{reaction_author_username}} reacted {{reaction_value}} on your comment: {{comment_name}}",
      "priority": 5
    }
  }
]
```

It is a list of notification definitions.

A notification definition is structured as follows:

- `name`: The name of the notification definition (used for logging)
- `event_type`: The event type to listen to (see tracim TLM documentation)
- `elements`: A list of elements to extract from the event
  - `name`: The name of the element (used for logging and templating)
  - `key`: The key of the element in the event relative to the `fields` element (see tracim TLM documentation)
- `filters`: A list of filters to apply to the event
  - `name`: The name of the filter (used for logging)
  - `key`: The key of the element in the event relative to the `fields` element (see tracim TLM documentation)
  - `match`: The match type to apply (see below)
  - `value`: The value to match against
- `notification`: The notification to send
  - `title`: The title of the notification
  - `message`: The message of the notification
  - `priority`: The priority of the notification (see below)


Title and message can be templated with the elements extracted from the event.
To do so, use the element name surrounded by double curly braces.

Filters allow messages in when the match is correct.

#### Match types

All match types are case-insensitive.

- `equal`: The value must be equal to the filter value
- `contains` The value must contain the filter value
- `starts_with` The value must start with the filter value
- `ends_with` The value must end with the filter value

To negate a match, prefix the match type with `not_`.


#### Notification priorities

See [Gotify documentation](https://gotify.net/docs/)
