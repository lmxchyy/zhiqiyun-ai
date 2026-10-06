# Disposable synthetic Python3.6/PG-driver test runtime; not a Carrier.
FROM python:3.6-alpine
RUN apk add --no-cache openssl bash libpq && \
    apk add --no-cache --virtual .build gcc musl-dev postgresql-dev && \
    pip install --no-cache-dir psycopg2==2.9.5 && apk del .build
ENV PYTHONDONTWRITEBYTECODE=1 ISSUE203_OWNED_APPROVAL_CONTAINER=1
WORKDIR /source
