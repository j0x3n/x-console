import type { RecordData } from "../../types/domain";
export const companyRowMeta: Record<
  string,
  [string | null, string | null, number]
> = {
  "Northvale Health": ["1:30 PM", "13:30", 1],
  "Kestrel Systems": ["Sep 26", "9 月 26 日", 2],
  "Lumen Pay": ["Sep 25", "9 月 25 日", 1],
  "Halcyon Robotics": ["10 AM", "10:00", 2],
  "Meridian Grid": ["Sep 29", "9 月 29 日", 1],
  "Veridian Farms": [null, null, 1],
  "Cobalt Forge": [null, null, 1],
  "Aurelian Bank": [null, null, 1],
  "Brightwell Labs": ["Sep 25", "9 月 25 日", 1],
  Wrenfield: [null, null, 1],
  "Harbor & Hale": [null, null, 1],
  "Oakline Freight": ["Sep 26", "9 月 26 日", 2],
  "Pinecrest Capital": [null, null, 1],
  "Palisade Security": ["Oct 1", "10 月 1 日", 1],
  "Solace Energy": [null, null, 1],
  Quillfeather: ["Sep 25", "9 月 25 日", 1],
  "Ironwood Analytics": ["Sep 28", "9 月 28 日", 0],
  Fernbrook: ["Sep 29", "9 月 29 日", 0],
  "Tidewater Logistics": ["Sep 30", "9 月 30 日", 0],
  "Orchard Street": ["Sep 25", "9 月 25 日", 0],
};

export const signalTone = (company: string) =>
  [
    "Veridian Farms",
    "Harbor & Hale",
    "Oakline Freight",
    "Orchard Street",
  ].includes(company)
    ? "risk"
    : [
          "Northvale Health",
          "Aurelian Bank",
          "Wrenfield",
          "Driftwood Studio",
        ].includes(company)
      ? "neutral"
      : "positive";

export const expansionAccounts = new Set([
  "Kestrel Systems",
  "Lumen Pay",
  "Meridian Grid",
  "Quillfeather",
  "Ironwood Analytics",
  "Palisade Security",
  "Halcyon Robotics",
  "Cobalt Forge",
]);

export const committeeContacts = new Set([
  "Carlos Mendes",
  "Helen Voss",
  "Marcus Oyelaran",
  "Claire Whitford",
  "Ben Hollis",
  "Dr. Amara Singh",
  "Priya Raman",
  "Owen Castellanos",
  "Elena Brandt",
  "Jun Watanabe",
  "Nikhil Rao",
  "Leah Okafor",
  "Dana Whitcombe",
  "Grace Lindahl",
  "Sam Okoye",
  "Tomás Reyes",
]);

export const coolingContacts = new Set([
  "Dr. Sofia Varga",
  "Luis Ortega",
  "Grace Lindahl",
  "Anika Rhodes",
  "Elena Brandt",
  "Ivy Chen",
  "Tariq Hassan",
  "Claire Whitford",
]);

export function matchesSavedView(
  row: RecordData,
  key: string,
  isPeople: boolean,
) {
  if (key === "All") return true;
  if (isPeople) {
    if (key === "Champions") return row.role === "Champion";
    if (key === "Committees") return committeeContacts.has(row.name);
    return coolingContacts.has(row.name);
  }
  if (key === "Mine") return row.owner === "jo";
  if (key === "Risk") return (row.change ?? 0) <= -11;
  return expansionAccounts.has(row.name);
}
