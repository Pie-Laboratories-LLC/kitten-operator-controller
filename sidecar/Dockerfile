FROM python:3.13-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY proxy.py .

RUN groupadd --gid 1000 morris && \
    useradd --uid 1000 --gid morris --home /app --shell /usr/sbin/nologin --no-log-init morris && \
    chown -R morris:morris /app

USER morris
ENV HOME=/tmp

EXPOSE 8000
CMD ["gunicorn", "--workers", "2", "--threads", "4", "--timeout", "30", "--bind", "0.0.0.0:8000", "proxy:app"]
