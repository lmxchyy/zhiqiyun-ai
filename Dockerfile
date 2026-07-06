FROM node:24-alpine AS web-build
WORKDIR /src/frontend-vue
COPY frontend-vue/package.json frontend-vue/package-lock.json ./
RUN npm ci
COPY frontend-vue ./
RUN npm run build

FROM node:24-alpine AS admin-build
WORKDIR /src/admin-vue
COPY admin-vue/package.json admin-vue/package-lock.json ./
RUN npm ci
COPY admin-vue ./
RUN npm run build

FROM golang:1.25-alpine AS api-build
WORKDIR /src/backend-go
COPY backend-go/go.mod backend-go/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN for i in 1 2 3 4 5; do go mod download && exit 0; echo "go mod download failed, retrying in 5s ($i/5)"; sleep 5; done; go mod download
COPY backend-go ./
RUN go build -o /out/xianzhi-api ./cmd/api

FROM alpine:3.20
WORKDIR /app
ARG INSTALL_SEEDANCE_SDK=false
RUN apk add --no-cache ca-certificates curl python3 py3-pip \
  && mkdir -p /app/seedance-python \
  && if [ "$INSTALL_SEEDANCE_SDK" = "true" ]; then \
    curl -fL --retry 5 --retry-delay 5 --retry-all-errors --connect-timeout 20 --max-time 180 "https://ecloud.10086.cn/api/query/maas/public/backend/model/link/aicc-sdk/python/download" -o /tmp/maas-seedance-sdk.zip \
    && python3 -c "import zipfile; zipfile.ZipFile('/tmp/maas-seedance-sdk.zip').extract('pythonSDK-0515/maas_seedance_sdk-1.0.0-py3-none-any.whl', '/tmp')" \
    && python3 -m pip install --break-system-packages --no-cache-dir --target /app/seedance-python /tmp/pythonSDK-0515/maas_seedance_sdk-1.0.0-py3-none-any.whl; \
  else \
    echo "Skipping optional Seedance SDK install"; \
  fi
COPY --from=api-build /out/xianzhi-api /app/xianzhi-api
COPY --from=web-build /src/frontend-vue/dist/build/h5 /app/frontend-vue/dist
COPY --from=admin-build /src/admin-vue/dist /app/admin-vue/dist
COPY backend-go/internal/provider/video/seedance_bridge.py /app/seedance_bridge.py
ENV PORT=3100
ENV XIANZHI_DATA_PATH=/app/data/store.json
ENV XIANZHI_STATIC_DIR=/app/frontend-vue/dist
ENV XIANZHI_ADMIN_STATIC_DIR=/app/admin-vue/dist
ENV CME_SEEDANCE_BRIDGE=/app/seedance_bridge.py
ENV CME_SEEDANCE_DEPS_PATH=/app/seedance-python
RUN mkdir -p /app/data \
  && printf '{"generationTasks":[],"assets":[],"counters":{}}\n' > /app/data/store.json \
  && adduser -D -H xianzhi \
  && chown -R xianzhi:xianzhi /app/data
USER xianzhi
EXPOSE 3100
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD curl -fsS http://127.0.0.1:3100/api/v1/health >/dev/null || exit 1
CMD ["/app/xianzhi-api"]
