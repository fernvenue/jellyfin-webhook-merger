# Jellyfin Webhook Merger

Merge the webhooks of Episodes based on queue, work with [jellyfin/jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook).

## Usage

This tool is a middleware that listens to a TCP port and receives requests from the Webhook Plugin. It completes batch pushes by creating queues, avoiding the need to notify each Episode individually. This ensures that notifications for Episodes are maintained without sending a large number of notifications due to a rapid update of a batch of Episodes.

Here is an example of pushing to Telegram. Download directly from Releases or build it yourself, and then run:

```
./jellyfin-webhook-merger --target-url "https://api.telegram.org/bot******/sendMessage" --additional-params '{"chat_id": "******","parse_mode": "html"}'
```

In the Webhook configuration page of Jellyfin, select **Add Generic Destination**, then you only need to check **Item Added** and tick **Episodes** in **Item Type**, and the **Template** must be:

```
{
  "SeriesId": "{{SeriesId}}",
  "SeriesName": "{{SeriesName}}",
  "SeasonNumber": {{SeasonNumber}},
  "EpisodeNumber": {{EpisodeNumber}},
  "EpisodeName": "{{Name}}"
}
```

And Webhook URL should be the address and port that `jellyfin-webhook-merger` listen to, by deafult it will be `http://[::1]:8520`.

Then you will receive a notification, like this:

```
📺 Episode update reminder: Series Season 1

Episode 1 EpisodeTitle...
Episode 2 EpisodeTitle...
Episode 3 EpisodeTitle...
...
```

### Custom Push Format (Or you would like to use another language to notify.)

If you don't want to use the default notification format, or if you want to use a language other than English for notifications, here, taking Chinese users as an example, we can use parameters like this:

```
--text-content "📺 <b>單集更新提醒:</b> <b>{{.SeriesName}}</b> <b>第 {{.SeasonNumber}} 季</b>\n" --episode-format "\n第 {{.EpisodeNumber}} 集"
```

Then we can receive notifications like this:

```
📺 單集更新提醒: 某劇 第 n 季

第 1 集 這一集名
第 2 集 這一集名
```

You can also define additional content to be added to the outgoing requests, fully customizing the received requests and outgoing requests. For specific details, please refer to the help message or the additional instructions in the documentation.

### Push to Telegram with Series Poster

Here we need to go to Jellyfin first, change the **Template** in **Item Type** like this: 

```
{
  "SeriesId": "{{SeriesId}}",
  "SeriesName": "{{SeriesName}}",
  "SeasonNumber": {{SeasonNumber}},
  "EpisodeNumber": {{EpisodeNumber}},
  "EpisodeName": "{{Name}}"
}
```

And we should run with:

```
./jellyfin-webhook-merger --target-url "https://api.telegram.org/bot******/sendPhoto" --text-key "caption" --additional-params "{\"chat_id\": \"******\", \"photo\": \"https://***/Items/{{.SeriesId}}/Images/Primary\", \"parse_mode\": \"html\"}"
```

Other parameters remain unchanged, and then you will receive notifications with Series images.

### Push to Telegram with Series Poster and Redirect Button

As with the previous part, we only need to make a small modification:

```
./jellyfin-webhook-merger --target-url "https://api.telegram.org/bot******/sendPhoto" --text-key "caption" --additional-params "{\"reply_markup\": {\"inline_keyboard\": [[{\"text\": \"Go Check it Out!\", \"url\": \"https://******/web/#/details?id={{.SeriesId}}&serverId=******\"}]]}, \"chat_id\": \"******\", \"photo\": \"https://***/Items/{{.SeriesId}}/Images/Primary\", \"parse_mode\": \"html\"}"
```

Then you will receive a notification with a jump button!

## Flags

- `-a`, `--listen-address`: Bind address, will use `::1` by default.
- `-p`, `--listen-port`: Bind port, will use TCP port `8520` by default.
- `-w`, `--wait-second`: Wait time in seconds before merging the notifications, will use `300` by default.
- `-r`, `--retry-count`: Number of times to retry sending the notification if the target URL does not return a 2xx response, waiting `--wait-second` seconds between attempts, will use `3` by default.
- `-k`, `--text-key`: Key used for the notification text in the JSON payload, will use `text` by default.
- `-t`, `--text-content`: Template for the notification text, supports variables like `{{.SeriesName}}` and `{{.SeasonNumber}}`.
- `-e`, `--episode-format`: Format for each episode's notification line, supports variables like `{{.EpisodeNumber}}` and `{{.EpisodeName}}`.
- `-u`, `--target-url`: Target URL to send the notification to. Must be specified.
- `-d`, `--additional-params`: Additional parameters in JSON format, supports variables like `{{.SeriesId}}`.
- `-c`, `--content-header`: Content type hint used when building the outgoing request, will use `text` by default.
- `-v`, `--version`: Print version and exit.
- `-h`, `--help`: Print help and exit.

## Configuration Options

Every parameter can also be set through an environment variable, which is useful when running in a container. Command-line flags take precedence over environment variables.

| Parameter                  | Environment Variable | Description                                                                 | Default Value                                               |
|-----------------------------|-----------------------|-----------------------------------------------------------------------------|-------------------------------------------------------------|
| `-w`, `--wait-second`       | `WAIT_SECOND`         | The wait time in seconds before merging the notifications.                   | 300                                                         |
| `-r`, `--retry-count`       | `RETRY_COUNT`         | The number of times to retry sending the notification if the target URL does not return a 2xx response, waiting `--wait-second` seconds between attempts. | 3                                                           |
| `-t`, `--text-content`      | `TEXT_CONTENT`        | The template for the notification text. You can use variables like `{{.SeriesName}}`, `{{.SeasonNumber}}`, and `{{.EpisodeName}}`. | `📺 <b>Episode update reminder:</b> <b>{{.SeriesName}}</b> <b>Season {{.SeasonNumber}}</b>\n` |
| `-k`, `--text-key`          | `TEXT_KEY`             | The key used for the notification text in the JSON payload, allowing flexibility in JSON structure. | `text`                                                      |
| `-e`, `--episode-format`    | `EPISODE_FORMAT`       | The format for each episode's notification. You can use variables like `{{.EpisodeNumber}}` and `{{.EpisodeName}}`. | `\nEpisode {{.EpisodeNumber}}`                               |
| `-u`, `--target-url`        | `TARGET_URL`           | The target URL to send the notification to.                                  | `""` (Must be specified)                                    |
| `-d`, `--additional-params` | `ADDITIONAL_PARAMS`    | Additional parameters in JSON format. Supports variables like `{{.SeriesId}}`. Example: `{"chat_id": "******", "photo": "https://example.com/{{.SeriesId}}"}`. | `{}` (Valid JSON format)                                    |
| `-a`, `--listen-address`    | `LISTEN_ADDRESS`       | The address to listen on. Defaults to `::1`.                                | `::1`                                                       |
| `-p`, `--listen-port`       | `LISTEN_PORT`          | The port to listen on. Defaults to `8520`.                                  | 8520                                                        |
| `-c`, `--content-header`    | `CONTENT_HEADER`       | Content type hint used when building the outgoing request.                  | `text`                                                       |
