import { UserPlus } from "lucide-react";

type Props = {
  label: string;
  onClick: () => void;
};

export function InviteButton({ label, onClick }: Props) {
  return (
    <button
      type="button"
      className="nav__theme"
      onClick={onClick}
      aria-label={label}
    >
      <UserPlus className="lucide" />
    </button>
  );
}
