import { Navbar } from "@/components/navbar";

export default function DefaultLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="bg-mesh-gradient relative flex min-h-screen flex-col">
      <Navbar />
      <main className="container mx-auto max-w-7xl flex-grow px-4 pt-4 sm:px-6 sm:pt-16">
        {children}
      </main>
    </div>
  );
}
