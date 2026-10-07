FROM node:22-alpine
WORKDIR /app
COPY package.json ./
COPY src ./src
COPY wsdl ./wsdl
USER node
EXPOSE 8088
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://localhost:8088/health || exit 1
CMD ["node", "src/server.js"]
