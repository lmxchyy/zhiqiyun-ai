# Synthetic verification runtime only; no production build or signing material.
FROM python:3.6-alpine
RUN apk add --no-cache openssl bash
ENV ISSUE203_OWNED_APPROVAL_CONTAINER=1 PYTHONDONTWRITEBYTECODE=1
WORKDIR /source
CMD ["python", "tests/issue203-approval-test.py"]
