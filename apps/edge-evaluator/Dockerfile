# Monorepo workspace dependency (@rollout-platform/contracts) -- build
# context must be the repo root, not this directory:
#   docker build -f apps/edge-evaluator/Dockerfile .

FROM node:22-alpine AS build
WORKDIR /repo
COPY . .
RUN npm ci
RUN npm run build --workspace @rollout-platform/contracts
RUN npm run build --workspace @rollout-platform/edge-evaluator

FROM node:22-alpine AS runtime
WORKDIR /repo
ENV NODE_ENV=production
COPY --from=build /repo ./
EXPOSE 4002
CMD ["node", "apps/edge-evaluator/dist/server.js"]
