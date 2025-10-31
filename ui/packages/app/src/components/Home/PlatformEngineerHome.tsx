import { useState } from 'react';
import {
  Grid,
  Box,
  Typography,
  Paper,
  InputAdornment,
  Dialog,
  DialogContent,
} from '@material-ui/core';
import { Content, Header, Page } from '@backstage/core-components';
import SearchIcon from '@material-ui/icons/Search';
import CloseIcon from '@material-ui/icons/Close';
import {
  HomePageStarredEntities,
  HomePageRecentlyVisited,
} from '@backstage/plugin-home';
import {
  SearchBar,
  SearchResult,
  SearchContextProvider,
} from '@backstage/plugin-search-react';
import { CatalogSearchResultListItem } from '@backstage/plugin-catalog';
import { TechDocsSearchResultListItem } from '@backstage/plugin-techdocs';
import {
  HomePagePlatformDetailsCard,
  InfrastructureWidget,
  DeveloperPortalWidget,
} from '@openchoreo/backstage-plugin-platform-engineer-core';
import { useStyles } from './styles';

const SearchModal = ({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) => {
  const classes = useStyles();

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      className={classes.searchModal}
    >
      <DialogContent className={classes.searchModalContent}>
        <SearchContextProvider>
          <Box className={classes.searchModalHeader}>
            <SearchBar />
            <CloseIcon className={classes.searchModalClose} onClick={onClose} />
          </Box>
          <Box className={classes.searchModalResults}>
            <SearchResult>
              <CatalogSearchResultListItem />
              <TechDocsSearchResultListItem />
            </SearchResult>
          </Box>
        </SearchContextProvider>
      </DialogContent>
    </Dialog>
  );
};

export const PlatformEngineerHome = () => {
  const classes = useStyles();
  const [searchModalOpen, setSearchModalOpen] = useState(false);

  const handleSearchClick = () => {
    setSearchModalOpen(true);
  };

  const handleSearchModalClose = () => {
    setSearchModalOpen(false);
  };

  return (
    <Page themeId="home">
      <Header
        title="Welcome to OpenChoreo"
        // subtitle="Your central hub for platform engineering, development tools, and infrastructure insights"
      />
      <Content>
        {/* Search Bar - Opens modal */}
        <Box className={classes.searchSection}>
          <Paper className={classes.searchPaper} onClick={handleSearchClick}>
            <InputAdornment position="start">
              <SearchIcon color="action" />
            </InputAdornment>
            <Typography className={classes.searchPlaceholder}>
              Search...
            </Typography>
          </Paper>
        </Box>

        {/* Search Modal */}
        <SearchModal open={searchModalOpen} onClose={handleSearchModalClose} />

        <Grid container spacing={3}>
          {/* Main Content */}
          <Grid item xs={12} md={8}>
            {/* Platform Metrics Section */}
            <Box className={classes.overviewSection}>
              <Typography variant="h3">Platform Overview</Typography>
              <Grid container className={classes.widgetContainer}>
                <Grid item xs={12} md={5} sm={12}>
                  <InfrastructureWidget />
                </Grid>
                <Grid item xs={12} md={5} sm={12}>
                  <DeveloperPortalWidget />
                </Grid>
              </Grid>
            </Box>

            {/* Platform Details Section */}
            <Box className={classes.platformDetailsSection}>
              <HomePagePlatformDetailsCard />
            </Box>
          </Grid>

          {/* Right Sidebar - Quick Access */}
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
