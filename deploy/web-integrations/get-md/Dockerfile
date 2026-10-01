FROM node:24.21.0-alpine3.23
WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev
COPY server.js ./
USER node
EXPOSE 3000
CMD ["node", "server.js"]
