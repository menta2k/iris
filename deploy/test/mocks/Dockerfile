# syntax=docker/dockerfile:1.7
FROM python:3.13-alpine

WORKDIR /app

# aiosmtpd: SMTP receiver. redis: only used by MOCK_BEHAVIOR=fbl, but bundled
# so a single image switches behavior via env at runtime.
RUN pip install --no-cache-dir aiosmtpd==1.4.6 redis==5.2.1

COPY mockmta.py /app/mockmta.py

# Privileged port 25 inside the container is fine — non-root in production
# would need port 2525; we expose 25 only inside the iris network.
EXPOSE 25

ENTRYPOINT ["python3", "/app/mockmta.py"]
