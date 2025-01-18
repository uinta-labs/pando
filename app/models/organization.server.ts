import { prisma } from "~/db.server";
import { User } from "@prisma/client";

export type { Organization } from "@prisma/client";

export async function createOrganization(name: string, userId: User["id"]) {
  return prisma.organization.create({
    data: {
      name,
      OrganizationUser: {
        create: {
          userId,
        },
      },
    },
  });
}

export async function getUserDefaultOrganization(userId: User["id"]) {
  const user = await prisma.user.findUnique({
    where: { id: userId },
    include: {
      OrganizationUser: {
        include: {
          organization: true,
        },
      },
    },
  });

  const organization = user?.OrganizationUser?.at(0)?.organization;
  if (!organization) {
    const userName = user?.givenName || user?.email;
    return await createOrganization(`${userName}'s Organization`, userId);
  }
  return organization;
}
