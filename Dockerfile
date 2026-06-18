FROM node:24-alpine AS web-build
WORKDIR /src/frontend-vue
COPY frontend-vue/package.json ./
RUN npm install
COPY frontend-vue ./
RUN npm run build

FROM golang:1.22-alpine AS api-build
WORKDIR /src/backend-go
COPY backend-go/go.mod ./
COPY backend-go ./
RUN go build -o /out/xianzhi-api ./cmd/api

FROM alpine:3.20
WORKDIR /app
COPY --from=api-build /out/xianzhi-api /app/xianzhi-api
COPY --from=web-build /src/frontend-vue/dist/build/h5 /app/frontend-vue/dist
COPY admin-web /app/admin-web
ENV PORT=3100
ENV XIANZHI_DATA_PATH=/app/data/store.json
ENV XIANZHI_STATIC_DIR=/app/frontend-vue/dist
ENV XIANZHI_ADMIN_STATIC_DIR=/app/admin-web
RUN mkdir -p /app/data \
  && printf '{"generationTasks":[],"assets":[],"counters":{}}\n' > /app/data/store.json \
  && adduser -D -H xianzhi \
  && chown -R xianzhi:xianzhi /app
USER xianzhi
EXPOSE 3100
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD wget -qO- http://127.0.0.1:3100/api/v1/health >/dev/null || exit 1
CMD ["/app/xianzhi-api"]
