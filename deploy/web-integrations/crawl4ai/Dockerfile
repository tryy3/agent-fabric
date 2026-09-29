FROM python:3.12.7-alpine3.20
WORKDIR /app
RUN pip install --no-cache-dir markdownify==0.14.1
COPY server.py ./
USER nobody
EXPOSE 11235
CMD ["python", "server.py"]
