-- DropForeignKey
ALTER TABLE "fleet" DROP CONSTRAINT "fleet_default_schedule_id_fkey";

-- AlterTable
ALTER TABLE "fleet" ALTER COLUMN "default_schedule_id" DROP NOT NULL;

-- AddForeignKey
ALTER TABLE "fleet" ADD CONSTRAINT "fleet_default_schedule_id_fkey" FOREIGN KEY ("default_schedule_id") REFERENCES "schedule"("id") ON DELETE SET NULL ON UPDATE CASCADE;
