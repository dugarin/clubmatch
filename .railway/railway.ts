import { defineRailway, image, project, service, volume } from "railway/iac";

const migrate = 'sh -c \'goose -dir /db/migrations postgres "$DATABASE_URL" up\'';

export default defineRailway(() => {
  const postgresVolume = volume("postgres-volume-SixC", {
    alerts: { usage: { "100": {}, "80": {}, "95": {} } },
    allowOnlineResize: true,
    region: "iad",
    sizeMB: 500,
  });
  const worker = service("worker", {
    build: { buildEnvironment: "V3", builder: "DOCKERFILE", dockerfilePath: "Dockerfile" },
    preDeploy: migrate,
    replicas: { iad: 1 },
  });
  const server = service("server", {
    build: { buildEnvironment: "V3", builder: "DOCKERFILE", dockerfilePath: "Dockerfile" },
    preDeploy: migrate,
    healthcheck: "/healthz",
    healthcheckTimeout: 300,
    replicas: { iad: 1 },
  });
  const postgres = service("Postgres", {
    source: image("postgres:16-alpine"),
    build: { buildEnvironment: "V3", builder: "RAILPACK" },
    replicas: { iad: 1 },
    networking: { privateNetworkEndpoint: "postgres" },
    volumeMounts: { "/var/lib/postgresql/data": postgresVolume },
  });

  return project("clubmatch", {
    variables: { managed: false },
    resources: [worker, server, postgres, postgresVolume],
  });
});
