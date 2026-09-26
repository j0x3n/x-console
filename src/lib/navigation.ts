import { companyRecords, peopleRecords } from "../data/workspace";

export const slug = (value: string) =>
  value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");

export const pathForView = (view: string) => {
  if (view === "Today") return "/";
  if (["Work", "Crew", "Settings"].includes(view))
    return `/${view.toLowerCase()}`;
  if (["Companies", "People", "Deals"].includes(view))
    return `/records/${view.toLowerCase()}`;
  if (companyRecords.some((item) => item.name === view))
    return `/records/companies/${slug(view)}`;
  if (peopleRecords.some((item) => item.name === view))
    return `/records/people/${slug(view)}`;
  return "/";
};

export const viewFromPath = (path: string) => {
  const parts = path.split("/").filter(Boolean);
  if (!parts.length) return "Today";
  if (parts[0] === "records") {
    if (parts.length === 2)
      return (
        { companies: "Companies", people: "People", deals: "Deals" }[
          parts[1]
        ] || "Today"
      );
    const records = parts[1] === "people" ? peopleRecords : companyRecords;
    return (
      records.find((item) => slug(item.name) === parts[2])?.name || "Today"
    );
  }
  return (
    { work: "Work", crew: "Crew", settings: "Settings" }[parts[0]] || "Today"
  );
};
