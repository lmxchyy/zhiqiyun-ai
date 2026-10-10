# NONOFFICIAL local CI packaging: all four freshly compiled source binaries.
# Not the production frontend/carrier image and never release authorization.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates python3 bash
WORKDIR /app
COPY xianzhi-api generation-worker smartvideo-worker video-backfill /app/
RUN adduser -D -H xianzhi
USER xianzhi
CMD ["/app/xianzhi-api"]
