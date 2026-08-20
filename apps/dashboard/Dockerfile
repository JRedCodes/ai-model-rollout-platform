# Monorepo workspace dependency (@rollout-platform/contracts) -- build
# context must be the repo root, not this directory:
#   docker build -f apps/dashboard/Dockerfile .
#
# Runtime is nginx (not Node) -- this is a static build, not a server. In
# an eventual AWS deploy this same dist/ output goes to S3 + CloudFront
# instead (see documents/DEPLOYMENT.md); nginx here is purely so
# docker-compose can run and curl this service like the other three for
# local parity testing.

FROM node:22-alpine AS build
WORKDIR /repo
COPY . .
RUN npm ci
RUN npm run build --workspace @rollout-platform/contracts
RUN npm run build --workspace @rollout-platform/dashboard

FROM nginx:1.27-alpine AS runtime
COPY apps/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/dashboard/dist /usr/share/nginx/html
EXPOSE 80
