import { prisma } from "app/db.server";

import type { Organization } from "@prisma/client";
import { v4 } from "uuid";
export type { Fleet } from "@prisma/client";

export async function createFleet(fleetName: string, organizationID: string) {
  const now = new Date();
  return prisma.fleet.create({
    data: {
      id: v4(),
      createdAt: now,
      updatedAt: now,
      name: fleetName,
      organizationId: organizationID,
    },
  });
}

export async function listFleetsForOrganization(
  organizationID: Organization["id"],
) {
  const currentFleets = await prisma.fleet.findMany({
    where: {
      organizationId: organizationID,
    },
  });
  if (currentFleets.length === 0) {
    return [await createFleet("Default Fleet", organizationID)];
  }
  return currentFleets;
}
