import {
  Activity,
  Blinds,
  Clapperboard,
  Fan,
  Gauge,
  House,
  Lightbulb,
  LightbulbOff,
  Lock,
  LockOpen,
  Power,
  ScrollText,
  Thermometer,
  ToggleLeft,
  ToggleRight,
  Tv,
  type LucideIcon,
} from "lucide-react";
import { domainOf, isOn, type EntityState } from "../logic";

function pick(entityId: string, state?: EntityState): LucideIcon {
  const on = state ? isOn(state) : false;
  switch (domainOf(entityId)) {
    case "light":
      return on ? Lightbulb : LightbulbOff;
    case "switch":
    case "input_boolean":
    case "automation":
      return on ? ToggleRight : ToggleLeft;
    case "fan":
      return Fan;
    case "climate":
      return Thermometer;
    case "sensor": {
      const cls = state?.attributes.device_class;
      return cls === "temperature" || state?.attributes.unit_of_measurement === "°C"
        ? Thermometer
        : Gauge;
    }
    case "binary_sensor":
      return Activity;
    case "lock":
      return state?.state === "locked" ? Lock : LockOpen;
    case "cover":
      return Blinds;
    case "scene":
      return Clapperboard;
    case "script":
      return ScrollText;
    case "button":
    case "input_button":
      return Power;
    case "media_player":
      return Tv;
    default:
      return House;
  }
}

export default function EntityIcon({
  entityId,
  state,
  size = 17,
}: {
  entityId: string;
  state?: EntityState;
  size?: number;
}) {
  const Icon = pick(entityId, state);
  return <Icon size={size} aria-hidden />;
}
