<script>
  import { route, href } from "./lib/router.js";
  import Dashboard from "./pages/Dashboard.svelte";
  import Leaderboard from "./pages/Leaderboard.svelte";
  import Users from "./pages/Users.svelte";
  import User from "./pages/User.svelte";
  import Gallery from "./pages/Gallery.svelte";

  const nav = [
    { path: "/", label: "Dashboard", match: "dashboard" },
    { path: "/leaderboard", label: "Leaderboard", match: "leaderboard" },
    { path: "/users", label: "Users", match: "users" },
    // { path: '/gallery', label: 'Gallery', match: 'gallery' },
  ];
</script>

<header class="topbar">
  <div class="container bar">
    <a class="brand" href={href("/")}>🍞 BreadBot</a>
    <nav>
      {#each nav as item}
        <a
          href={href(item.path)}
          class:active={$route.name === item.match ||
            (item.match === "users" && $route.name === "user")}>{item.label}</a
        >
      {/each}
    </nav>
  </div>
</header>

<main class="container">
  {#if $route.name === "dashboard"}
    <Dashboard />
  {:else if $route.name === "leaderboard"}
    <Leaderboard />
  {:else if $route.name === "users"}
    <Users />
  {:else if $route.name === "user"}
    <User id={$route.params.id} />
    <!-- {:else if $route.name === "gallery"}
    <Gallery /> -->
  {/if}
</main>

<style>
  .topbar {
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    z-index: 10;
  }
  .bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1.25rem;
    gap: 1rem;
    flex-wrap: wrap;
  }
  .brand {
    font-weight: 700;
    font-size: 1.15rem;
    color: var(--text);
  }
  .brand:hover {
    text-decoration: none;
  }
  nav {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }
  nav a {
    padding: 0.35rem 0.75rem;
    border-radius: 8px;
    color: var(--text-muted);
  }
  nav a:hover {
    background: var(--surface-2);
    text-decoration: none;
  }
  nav a.active {
    color: #fff;
    background: var(--accent);
  }
</style>
