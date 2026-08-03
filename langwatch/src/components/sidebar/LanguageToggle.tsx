import { Box, HStack, Text } from "@chakra-ui/react";
import { LuLanguages } from "react-icons/lu";
import { type BarenaLocale, useBarenaI18n } from "~/features/barena/i18n";
import { MENU_ITEM_HEIGHT } from "./SideMenuLink";

export type LanguageToggleProps = {
  showLabel?: boolean;
};

const languageOptions: {
  value: BarenaLocale;
  shortLabel: string;
  message: "English" | "Chinese";
}[] = [
  { value: "zh-CN", shortLabel: "中", message: "Chinese" },
  { value: "en", shortLabel: "EN", message: "English" },
];

export const LanguageToggle = ({ showLabel = true }: LanguageToggleProps) => {
  const { locale, setLocale, t } = useBarenaI18n();

  if (!showLabel) {
    const nextLocale = locale === "zh-CN" ? "en" : "zh-CN";
    const nextLanguage = nextLocale === "zh-CN" ? t("Chinese") : t("English");
    return (
      <Box width="full" py={1}>
        <HStack
          as="button"
          width="auto"
          height={MENU_ITEM_HEIGHT}
          paddingX={3}
          borderRadius="lg"
          color="fg.subtle"
          cursor="pointer"
          _hover={{ color: "fg", backgroundColor: "nav.bgHover" }}
          aria-label={t("Switch language to {language}", {
            language: nextLanguage,
          })}
          onClick={() => setLocale(nextLocale)}
        >
          <LuLanguages size={16} />
        </HStack>
      </Box>
    );
  }

  return (
    <HStack
      width="full"
      height={MENU_ITEM_HEIGHT}
      paddingX={3}
      justify="space-between"
      aria-label={t("Language")}
    >
      <HStack gap={3} color="nav.fg">
        <LuLanguages size={16} color="var(--chakra-colors-nav-fg-muted)" />
        <Text fontSize="14px">{t("Language")}</Text>
      </HStack>
      <HStack
        role="radiogroup"
        aria-label={t("Language")}
        gap={0.5}
        padding={0.5}
        borderRadius="md"
        bg="bg.muted"
      >
        {languageOptions.map((option) => {
          const selected = option.value === locale;
          return (
            <Box
              key={option.value}
              as="button"
              role="radio"
              aria-checked={selected}
              aria-label={t(option.message)}
              minWidth="28px"
              height="23px"
              paddingX={1.5}
              borderRadius="sm"
              bg={selected ? "bg.surface" : "transparent"}
              color={selected ? "fg" : "fg.muted"}
              boxShadow={selected ? "xs" : "none"}
              fontSize="11px"
              fontWeight={selected ? "semibold" : "medium"}
              cursor="pointer"
              transition="all 0.15s ease"
              _hover={{ color: "fg" }}
              onClick={() => setLocale(option.value)}
            >
              {option.shortLabel}
            </Box>
          );
        })}
      </HStack>
    </HStack>
  );
};
