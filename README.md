# HLS HTTP to HTTPS Proxy

This is a simple Go-based proxy that converts HLS stream URLs from HTTP to HTTPS. It's designed to be run in a lean Docker container.

## Build the Docker Image

To build the Docker image, run the following command in the root of the project:

```sh
docker build -t hls-proxy .
```

## Run the Docker Container

To run the Docker container, use the following command:

```sh
docker run -p 8080:8080 hls-proxy
```

This will start the proxy on port 8080.

## Usage

To use the proxy, you need to provide the URL of the HLS playlist as a query parameter. The proxy will fetch the playlist, rewrite the URLs of the segments to also go through the proxy, and serve the modified playlist.

### Example with curl

```sh
curl "http://localhost:8080/?url=your-hls-playlist-url.m3u8"
```

Replace `your-hls-playlist-url.m3u8` with the actual URL of your HLS playlist.

### Example with a media player

You can use a media player like VLC or a web-based HLS player to play the stream. Just provide the proxied URL to the player:

```
http://localhost:8080/?url=your-hls-playlist-url.m3u8
```
