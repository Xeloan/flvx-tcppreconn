import React from "react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/shadcn-bridge/heroui/button";
import { BackIcon } from "@/components/icons";
import { BrandLogo } from "@/components/brand-logo";
import { siteConfig } from "@/config/site";
import { useScrollTopOnPathChange } from "@/hooks/useScrollTopOnPathChange";

export default function H5SimpleLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const navigate = useNavigate();

  useScrollTopOnPathChange();

  const handleBack = () => {
    navigate("/profile");
  };

  return (
    <div className="bg-mesh-gradient flex min-h-screen flex-col">
      {/* 顶部导航栏 */}
      <header className="bg-background/85 dark:bg-background/80 backdrop-blur-xl shadow-sm border-b border-gray-200 dark:border-gray-600 h-14 safe-top flex-shrink-0 flex items-center justify-between px-4 relative z-10">
        <div className="flex items-center gap-2">
          <Button isIconOnly size="sm" variant="light" onPress={handleBack}>
            <BackIcon className="w-5 h-5" />
          </Button>
          <BrandLogo size={20} />
          <h1 className="text-sm font-bold text-foreground">
            {siteConfig.name}
          </h1>
        </div>

        <div className="flex items-center gap-2" />
      </header>

      {/* 主内容区域 */}
      <main className="flex-1 bg-transparent pb-0">{children}</main>
    </div>
  );
}
