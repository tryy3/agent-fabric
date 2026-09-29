FROM node:22.19.0-alpine3.20
WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev
COPY server.js ./
USER node
EXPOSE 3000
CMD ["node", "server.js"]
