/*
  Warnings:

  - You are about to drop the column `scheduleId` on the `container` table. All the data in the column will be lost.

*/
-- DropForeignKey
ALTER TABLE "container" DROP CONSTRAINT "container_scheduleId_fkey";

-- AlterTable
ALTER TABLE "container" DROP COLUMN "scheduleId",
ADD COLUMN     "schedule_id" UUID;

-- AddForeignKey
ALTER TABLE "container" ADD CONSTRAINT "container_schedule_id_fkey" FOREIGN KEY ("schedule_id") REFERENCES "schedule"("id") ON DELETE SET NULL ON UPDATE CASCADE;
