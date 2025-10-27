import { Grid, Box, Typography } from '@material-ui/core';
import { Content, Header, Page } from '@backstage/core-components';
import {
  HomePageStarredEntities,
  HomePageRecentlyVisited,
} from '@backstage/plugin-home';
import {
  MyProjectsWidget,
  QuickActionsSection,
} from '@openchoreo/backstage-plugin';
import { useStyles } from './styles';

export const DeveloperHome = () => {
  const classes = useStyles();

  return (
    <Page themeId="home">
      <Header title="Welcome to OpenChoreo" />
      <Content>
        <Grid container spacing={3}>
          {/* Main Content */}
          <Grid item xs={12} md={8}>
            {/* My Projects Widget Section */}
            <Box className={classes.overviewSection}>
              <Typography variant="h3">Overview</Typography>
              <Grid container className={classes.widgetContainer}>
                <Grid item xs={12} md={5} sm={12}>
                  <MyProjectsWidget />
                </Grid>
              </Grid>
            </Box>

            {/* Quick Actions Section */}
            <QuickActionsSection />
          </Grid>

          {/* Right Sidebar - Recent Activity */}
          <Grid item xs={12} md={4}>
            <Grid container justifyContent="flex-end">
              <Grid item md={10} xs={12} className={classes.sidebarSection}>
                <Typography variant="h4" color="secondary">
                  Recent Activity
                </Typography>

                <Box className={classes.sidebarWidget}>
                  <HomePageStarredEntities />
                </Box>

                <Box className={classes.sidebarWidget}>
                  <HomePageRecentlyVisited />
                </Box>
              </Grid>
            </Grid>
          </Grid>
        </Grid>
      </Content>
    </Page>
  );
};
